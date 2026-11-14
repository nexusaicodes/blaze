package dodge

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/sakshamsaxena/blaze/internal/logger"
)

// posIndex maps a byte position in a repo file to the symbol whose
// declaration span contains it.
type posIndex struct {
	root  string            // symlink-resolved worktree root
	files map[string][]span // repo-relative path → spans sorted by start
}

// resolve returns the symbol whose span contains pos, or "" if none.
func (ix *posIndex) resolve(fset *token.FileSet, pos token.Pos) SymbolID {
	if !pos.IsValid() {
		return ""
	}
	p := fset.Position(pos)
	rel, err := relToRoot(ix.root, p.Filename)
	if err != nil {
		return ""
	}
	spans := ix.files[rel]
	i := sort.Search(len(spans), func(i int) bool { return spans[i].start > p.Offset })
	if i == 0 {
		return ""
	}
	s := spans[i-1]
	if p.Offset < s.end {
		return s.id
	}
	return ""
}

// relToRoot resolves symlinks in filename and root and makes filename
// relative to root, with forward slashes. It errors when filename is not
// under root.
func relToRoot(root, filename string) (string, error) {
	resolved, err := filepath.EvalSymlinks(filename)
	if err != nil {
		resolved = filename
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s is outside %s", filename, root)
	}
	return filepath.ToSlash(rel), nil
}

// sortPackagesForBuild orders packages so that plain packages (ID without
// " [") are processed before their test variants, and by ID within each
// group. Plain packages form one consistent type universe, so processing
// them first gives the best implements results.
func sortPackagesForBuild(pkgs []*packages.Package) {
	sort.SliceStable(pkgs, func(i, j int) bool {
		vi := strings.Contains(pkgs[i].ID, " [")
		vj := strings.Contains(pkgs[j].ID, " [")
		if vi != vj {
			return vj
		}
		return pkgs[i].ID < pkgs[j].ID
	})
}

// Build loads the module at in.RepoRoot/in.ModuleDir with full type
// information and returns its symbol graph.
func (p *goProvider) Build(ctx context.Context, in BuildInput) (*Graph, error) {
	log := logger.Get().With().Str("component", "dodge").Logger()

	root, err := filepath.EvalSymlinks(in.RepoRoot)
	if err != nil {
		return nil, fmt.Errorf("build graph: resolve root: %w", err)
	}

	dir := filepath.Join(in.RepoRoot, in.ModuleDir)
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedTypes | packages.NeedTypesSizes |
			packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedModule,
		Dir:     dir,
		Tests:   true,
		Context: ctx,
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("load packages: %w", err)
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no Go packages found in %s", dir)
	}

	var modulePath string
	for _, pk := range pkgs {
		if pk.Module != nil {
			modulePath = pk.Module.Path
			break
		}
	}

	sortPackagesForBuild(pkgs)

	g := newGraph(modulePath)
	ix := &posIndex{root: root, files: map[string][]span{}}
	objs := map[SymbolID]types.Object{}
	seen := map[string]bool{}

	type workItem struct {
		pkg     *packages.Package
		file    *ast.File
		relPath string
	}
	var workList []workItem

	for _, pk := range pkgs {
		if len(pk.Errors) > 0 && len(g.Warnings) < 10 {
			g.Warnings = append(g.Warnings, fmt.Sprintf("%s: %s", pk.PkgPath, pk.Errors[0]))
		}
		if pk.TypesInfo == nil {
			continue
		}
		for _, f := range pk.Syntax {
			filename := pk.Fset.Position(f.Package).Filename
			if seen[filename] {
				continue
			}
			relPath, rerr := relToRoot(root, filename)
			if rerr != nil {
				continue
			}
			seen[filename] = true

			if expected := packagePath(relPath, modulePath, in.ModuleDir, f.Name.Name); expected != pk.PkgPath {
				log.Warn().Str("file", relPath).Str("expected", expected).Str("got", pk.PkgPath).
					Msg("package path mismatch")
			}

			src, rerr := os.ReadFile(filepath.Join(root, relPath))
			if rerr != nil {
				return nil, fmt.Errorf("read %s: %w", relPath, rerr)
			}

			syms, spans, idents := extractAll(pk.Fset, f, src, relPath, pk.PkgPath)
			for i, sym := range syms {
				sym.Resolved = true
				if !g.addSymbol(sym) {
					g.Warnings = append(g.Warnings, fmt.Sprintf("duplicate symbol id %s", sym.ID))
					continue
				}
				if obj := pk.TypesInfo.Defs[idents[i]]; obj != nil {
					objs[sym.ID] = obj
				}
			}
			sort.Slice(spans, func(a, b int) bool { return spans[a].start < spans[b].start })
			ix.files[relPath] = spans
			workList = append(workList, workItem{pkg: pk, file: f, relPath: relPath})
		}
	}

	for _, w := range workList {
		buildRefs(g, ix, w.pkg, w.file, w.relPath)
	}
	buildImplements(g, objs, log)

	g.finish()
	return g, nil
}
