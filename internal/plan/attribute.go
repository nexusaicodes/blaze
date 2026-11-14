package plan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sakshamsaxena/blaze/internal/diff"
	"github.com/sakshamsaxena/blaze/internal/dodge"
)

// hashID returns prefix followed by the first 12 hex characters of
// sha256(key), the scheme used for both Item.ID and Thread.ID.
func hashID(prefix, key string) string {
	sum := sha256.Sum256([]byte(key))
	return prefix + hex.EncodeToString(sum[:])[:12]
}

// contentHash is Item.ContentHash: hex SHA-256 of key + "\n" followed by,
// for every slice in order and every line in order, "<c><Text>\n" where c
// is ' ', '+' or '-'.
func contentHash(key string, slices []Slice) string {
	h := sha256.New()
	h.Write([]byte(key))
	h.Write([]byte("\n"))
	for _, sl := range slices {
		for _, l := range sl.Lines {
			c := byte(' ')
			switch l.Kind {
			case diff.LineAdd:
				c = '+'
			case diff.LineDel:
				c = '-'
			}
			h.Write([]byte{c})
			h.Write([]byte(l.Text))
			h.Write([]byte("\n"))
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// preambleKey returns the Item.Key of the preamble item for a Go file at
// path.
func preambleKey(path string) string {
	return "file:" + path + "#preamble"
}

// isGoFile reports whether fd is attributed symbol by symbol rather than
// as a single file item: not binary, Path() ends in ".go", and Path() is
// inside moduleDir.
func isGoFile(fd diff.FileDiff, moduleDir string) bool {
	if fd.Binary {
		return false
	}
	if !strings.HasSuffix(fd.Path(), ".go") {
		return false
	}
	return pathInModuleDir(fd.Path(), moduleDir)
}

// pathInModuleDir reports whether repo-relative path p lies inside
// moduleDir ("." or "" means the whole repository).
func pathInModuleDir(p, moduleDir string) bool {
	if moduleDir == "." || moduleDir == "" {
		return true
	}
	return p == moduleDir || strings.HasPrefix(p, moduleDir+"/")
}

// symbolCollection holds the symbols gathered from both sides of the diff
// for every Go file, as described in "Collect symbols for both sides".
type symbolCollection struct {
	headByID   map[dodge.SymbolID]*dodge.Symbol
	baseByID   map[dodge.SymbolID]*dodge.Symbol
	headByFile map[string][]*dodge.Symbol
	baseByFile map[string][]*dodge.Symbol
}

// attribute implements step 1 of the plan: every changed line is assigned
// to exactly one item. It returns the items keyed by Item.Key, plus
// warnings collected while loading symbols for files the Go toolchain did
// not resolve, or could not parse.
func attribute(ctx context.Context, in Input) (map[string]*Item, []string, error) {
	col, warnings, err := collectSymbols(ctx, in)
	if err != nil {
		return nil, warnings, err
	}

	items := map[string]*Item{}
	for _, fd := range in.Files {
		if isGoFile(fd, in.ModuleDir) {
			attributeGoFile(items, fd, col)
		} else {
			attributeFileItem(items, fd)
		}
	}

	for key, item := range items {
		item.Added, item.Deleted = countAddDel(item.Slices)
		item.ContentHash = contentHash(key, item.Slices)
		switch item.Kind {
		case ItemSymbol:
			finalizeSymbolItem(item, key, col)
		case ItemPreamble:
			finalizePreambleItem(item, key, col)
		}
	}

	return items, warnings, nil
}

// collectSymbols gathers head and base symbols for every Go file in
// in.Files, building the lookup maps step 1 attributes lines against.
func collectSymbols(ctx context.Context, in Input) (symbolCollection, []string, error) {
	col := symbolCollection{
		headByID:   map[dodge.SymbolID]*dodge.Symbol{},
		baseByID:   map[dodge.SymbolID]*dodge.Symbol{},
		headByFile: map[string][]*dodge.Symbol{},
		baseByFile: map[string][]*dodge.Symbol{},
	}
	var warnings []string

	for _, fd := range in.Files {
		if !isGoFile(fd, in.ModuleDir) {
			continue
		}

		if fd.NewPath != "" {
			syms := in.Graph.SymbolsInFile(fd.NewPath)
			if len(syms) == 0 {
				warnings = append(warnings, fmt.Sprintf("%s: not loaded by the Go toolchain (build constraints?); shown without blast radius", fd.NewPath))
				src, err := os.ReadFile(filepath.Join(in.Worktree, fd.NewPath))
				if err != nil {
					return symbolCollection{}, warnings, fmt.Errorf("read head %s: %w", fd.NewPath, err)
				}
				parsed, err := in.Provider.ParseFile(dodge.ParseInput{
					Path:       fd.NewPath,
					Src:        src,
					ModulePath: in.Graph.ModulePath,
					ModuleDir:  in.ModuleDir,
				})
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("%s: %s", fd.NewPath, err))
				} else {
					syms = parsed
				}
			}
			col.headByFile[fd.NewPath] = syms
			for _, s := range syms {
				col.headByID[s.ID] = s
			}
		}

		if fd.OldPath != "" && fd.Status != diff.StatusAdded {
			src, err := in.Git.Show(ctx, in.Target.BaseSHA, fd.OldPath)
			if err != nil {
				return symbolCollection{}, warnings, fmt.Errorf("read base %s: %w", fd.OldPath, err)
			}
			syms, err := in.Provider.ParseFile(dodge.ParseInput{
				Path:       fd.OldPath,
				Src:        src,
				ModulePath: in.Graph.ModulePath,
				ModuleDir:  in.ModuleDir,
			})
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %s", fd.OldPath, err))
				syms = nil
			}
			col.baseByFile[fd.OldPath] = syms
			for _, s := range syms {
				col.baseByID[s.ID] = s
			}
		}
	}

	return col, warnings, nil
}

// ownerAt returns the symbol in syms whose span contains line, or nil if
// none does.
func ownerAt(syms []*dodge.Symbol, line int) *dodge.Symbol {
	for _, s := range syms {
		if s.StartLine <= line && line <= s.EndLine {
			return s
		}
	}
	return nil
}

// attributeGoFile cuts every hunk of fd into slices owned by symbol or
// preamble items, per the owner and blank-line rules, and appends each
// slice to the item named by its owner key in items.
func attributeGoFile(items map[string]*Item, fd diff.FileDiff, col symbolCollection) {
	headSyms := col.headByFile[fd.NewPath]
	baseSyms := col.baseByFile[fd.OldPath]
	preamble := preambleKey(fd.Path())

	for _, h := range fd.Hunks {
		owners := make([]string, len(h.Lines))
		for i, l := range h.Lines {
			owners[i] = ownerKey(l, headSyms, baseSyms, preamble)
		}
		applyBlankLineRule(h.Lines, owners, preamble)

		start := 0
		for i := 1; i <= len(h.Lines); i++ {
			if i == len(h.Lines) || owners[i] != owners[start] {
				run := h.Lines[start:i]
				if hasChange(run) {
					appendSlice(items, owners[start], fd.Path(), run)
				}
				start = i
			}
		}
	}
}

// ownerKey returns the owner key for one diff line, per the "Owner of a
// diff line" table: the head symbol for add and context lines, the base
// symbol for del lines, or preamble when no symbol contains the line.
func ownerKey(l diff.Line, headSyms, baseSyms []*dodge.Symbol, preamble string) string {
	if l.Kind == diff.LineDel {
		if s := ownerAt(baseSyms, l.OldNo); s != nil {
			return string(s.ID)
		}
		return preamble
	}
	if s := ownerAt(headSyms, l.NewNo); s != nil {
		return string(s.ID)
	}
	return preamble
}

// applyBlankLineRule reassigns the owner of every add or del line in lines
// whose owner is preamble and whose text is blank: it takes the owner of
// the nearest following line in lines with a symbol owner, else the
// nearest preceding one, else it stays with the preamble. The search is
// done against a snapshot of owners taken before any reassignment, so two
// adjacent blank lines resolve independently of each other.
func applyBlankLineRule(lines []diff.Line, owners []string, preamble string) {
	orig := append([]string{}, owners...)
	for i, l := range lines {
		if orig[i] != preamble || l.Kind == diff.LineContext {
			continue
		}
		if strings.TrimSpace(l.Text) != "" {
			continue
		}

		assigned := false
		for j := i + 1; j < len(lines); j++ {
			if orig[j] != preamble {
				owners[i] = orig[j]
				assigned = true
				break
			}
		}
		if assigned {
			continue
		}
		for j := i - 1; j >= 0; j-- {
			if orig[j] != preamble {
				owners[i] = orig[j]
				break
			}
		}
	}
}

// hasChange reports whether run contains at least one add or del line.
func hasChange(run []diff.Line) bool {
	for _, l := range run {
		if l.Kind != diff.LineContext {
			return true
		}
	}
	return false
}

// sliceStarts returns the first non-zero OldNo and the first non-zero
// NewNo in lines, each 0 if there is none.
func sliceStarts(lines []diff.Line) (oldStart, newStart int) {
	for _, l := range lines {
		if oldStart == 0 && l.OldNo != 0 {
			oldStart = l.OldNo
		}
		if newStart == 0 && l.NewNo != 0 {
			newStart = l.NewNo
		}
	}
	return oldStart, newStart
}

// countAddDel counts the add and del lines over all of slices.
func countAddDel(slices []Slice) (added, deleted int) {
	for _, sl := range slices {
		for _, l := range sl.Lines {
			switch l.Kind {
			case diff.LineAdd:
				added++
			case diff.LineDel:
				deleted++
			}
		}
	}
	return added, deleted
}

// appendSlice appends one Slice built from lines to the item named by key,
// creating it (as a symbol or preamble item) if this is its first slice.
func appendSlice(items map[string]*Item, key, path string, lines []diff.Line) {
	item, ok := items[key]
	if !ok {
		kind := ItemSymbol
		if strings.HasSuffix(key, "#preamble") {
			kind = ItemPreamble
		}
		item = &Item{Key: key, ID: hashID("i", key), Kind: kind}
		items[key] = item
	}
	oldStart, newStart := sliceStarts(lines)
	item.Slices = append(item.Slices, Slice{
		Path:     path,
		OldStart: oldStart,
		NewStart: newStart,
		Lines:    append([]diff.Line{}, lines...),
	})
}

// finalizeSymbolItem fills in the fields of a symbol item that depend on
// which side(s) of the diff its symbol ID appears on.
func finalizeSymbolItem(item *Item, key string, col symbolCollection) {
	head, hasHead := col.headByID[dodge.SymbolID(key)]
	base, hasBase := col.baseByID[dodge.SymbolID(key)]

	switch {
	case hasHead && hasBase:
		item.Status = ChangeModified
		item.Symbol = head
		item.SignatureChanged = base.Signature != head.Signature
		item.formatOnly = base.BodyHash == head.BodyHash
	case hasHead:
		item.Status = ChangeAdded
		item.Symbol = head
	case hasBase:
		item.Status = ChangeDeleted
		item.Symbol = base
	default:
		return
	}
	item.Path = item.Symbol.File
	item.Title = item.Symbol.PkgName + "." + item.Symbol.Name
}

// finalizePreambleItem fills in the fields of a preamble item: its path,
// title, status, and whether the file it belongs to is generated.
func finalizePreambleItem(item *Item, key string, col symbolCollection) {
	path := strings.TrimSuffix(strings.TrimPrefix(key, "file:"), "#preamble")
	item.Path = path
	item.Title = path + " (imports and header)"
	item.Status = ChangeModified

	syms := col.headByFile[path]
	if len(syms) == 0 {
		syms = col.baseByFile[path]
	}
	if len(syms) > 0 {
		item.fileGenerated = syms[0].Generated
	}
}

// attributeFileItem builds the single Item for a non-Go FileDiff: one
// slice per hunk, holding all of that hunk's lines.
func attributeFileItem(items map[string]*Item, fd diff.FileDiff) {
	key := "file:" + fd.Path()
	item := &Item{
		ID:     hashID("i", key),
		Key:    key,
		Kind:   ItemFile,
		Title:  fd.Path(),
		Path:   fd.Path(),
		Status: fileItemStatus(fd.Status),
		Binary: fd.Binary,
	}
	if item.Status == ChangeRenamed {
		item.OldPath = fd.OldPath
	}
	for _, h := range fd.Hunks {
		oldStart, newStart := sliceStarts(h.Lines)
		item.Slices = append(item.Slices, Slice{
			Path:     fd.Path(),
			OldStart: oldStart,
			NewStart: newStart,
			Lines:    append([]diff.Line{}, h.Lines...),
		})
	}
	items[key] = item
}

// fileItemStatus maps a diff.FileStatus to the ChangeStatus of the file
// item it produces: added and deleted pass through, renamed and copied
// become renamed, everything else becomes modified.
func fileItemStatus(s diff.FileStatus) ChangeStatus {
	switch s {
	case diff.StatusAdded:
		return ChangeAdded
	case diff.StatusDeleted:
		return ChangeDeleted
	case diff.StatusRenamed, diff.StatusCopied:
		return ChangeRenamed
	default:
		return ChangeModified
	}
}
