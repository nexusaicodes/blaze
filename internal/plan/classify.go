package plan

import (
	"path"
	"strings"

	"github.com/sakshamsaxena/blaze/internal/dodge"
)

// lockfileNames are the file names classified as a generated lockfile.
var lockfileNames = map[string]bool{
	"go.sum":            true,
	"package-lock.json": true,
	"yarn.lock":         true,
	"pnpm-lock.yaml":    true,
	"Cargo.lock":        true,
	"poetry.lock":       true,
	"Gemfile.lock":      true,
	"composer.lock":     true,
}

// generatedGlobs are base-name globs classified as a generated file.
var generatedGlobs = []string{"*.pb.go", "*_gen.go", "*.gen.go", "zz_generated*", "*.min.js", "*.snap"}

// testGlobs are base-name globs classified as a test file or test data.
var testGlobs = []string{"*_test.*", "*.test.*", "*.spec.*"}

// schemaExts are file extensions classified as a schema or API definition.
var schemaExts = map[string]bool{
	".proto": true, ".sql": true, ".graphql": true, ".gql": true, ".thrift": true, ".avsc": true,
}

// docExts are file extensions classified as documentation.
var docExts = map[string]bool{
	".md": true, ".mdx": true, ".rst": true, ".txt": true, ".adoc": true,
}

// configExts are file extensions classified as configuration.
var configExts = map[string]bool{
	".yaml": true, ".yml": true, ".json": true, ".toml": true, ".ini": true, ".tf": true, ".hcl": true,
}

// configBaseNames are exact file names classified as configuration.
var configBaseNames = map[string]bool{
	"Makefile": true, ".gitignore": true, ".editorconfig": true,
}

// base returns the last path element of p.
func base(p string) string {
	return path.Base(p)
}

// ext returns the lower-cased extension of p, including the leading dot.
func ext(p string) string {
	return strings.ToLower(path.Ext(p))
}

// seg reports whether some slash-separated element of p equals x.
func seg(p, x string) bool {
	for _, part := range strings.Split(p, "/") {
		if part == x {
			return true
		}
	}
	return false
}

// firstSeg returns the first slash-separated element of p.
func firstSeg(p string) string {
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return p
}

// matchAny reports whether base name b matches any of patterns, using
// path.Match.
func matchAny(b string, patterns ...string) bool {
	for _, pat := range patterns {
		if ok, _ := path.Match(pat, b); ok {
			return true
		}
	}
	return false
}

// classify returns item's Role and the reason text for it, trying the
// rules of 05-plan.md's classification table in order; the first match
// wins.
func classify(item *Item) (Role, string) {
	switch item.Kind {
	case ItemSymbol:
		return classifySymbol(item)
	case ItemPreamble:
		return classifyPreamble(item)
	default:
		return classifyFile(item)
	}
}

// classifySymbol classifies a symbol item: rules 1, 6, 8, 10-15 of the
// classification table.
func classifySymbol(item *Item) (Role, string) {
	sym := item.Symbol

	if sym.Generated {
		return RoleGenerated, "generated file"
	}
	if item.formatOnly {
		return RoleMechanical, "comments or formatting only"
	}
	if sym.IsTest {
		return RoleTest, "test code"
	}
	if sym.Kind == dodge.KindType {
		return RoleContract, "type declaration"
	}
	if (sym.Kind == dodge.KindFunc || sym.Kind == dodge.KindMethod) && item.SignatureChanged {
		return RoleContract, "signature changed"
	}
	if item.Status == ChangeDeleted && sym.Exported {
		return RoleContract, "exported symbol removed"
	}
	if (sym.Kind == dodge.KindVar || sym.Kind == dodge.KindConst) && sym.Exported {
		return RoleContract, "exported value"
	}
	if sym.PkgName == "main" || seg(sym.File, "cmd") || sym.Name == "init" || sym.Name == "main" {
		return RoleWiring, "entry point or wiring"
	}
	return RoleLogic, "implementation"
}

// classifyPreamble classifies a preamble item: rules 1 and 5 of the
// classification table.
func classifyPreamble(item *Item) (Role, string) {
	if item.fileGenerated {
		return RoleGenerated, "generated file"
	}
	return RoleMechanical, "imports and file header"
}

// classifyFile classifies a file item: rules 2-4, 7, 9, 16-19 of the
// classification table.
func classifyFile(item *Item) (Role, string) {
	p := item.Path
	b := base(p)
	e := ext(p)

	if seg(p, "vendor") || seg(p, "node_modules") {
		return RoleGenerated, "vendored dependency"
	}
	if lockfileNames[b] {
		return RoleGenerated, "lockfile"
	}
	if matchAny(b, generatedGlobs...) {
		return RoleGenerated, "generated file"
	}
	if len(item.Slices) == 0 && !item.Binary && item.Status == ChangeRenamed {
		return RoleMechanical, "renamed without changes"
	}
	if seg(p, "testdata") || matchAny(b, testGlobs...) {
		return RoleTest, "test file or test data"
	}
	if schemaExts[e] || seg(p, "migrations") || seg(p, "migration") || strings.HasPrefix(b, "openapi") || strings.HasPrefix(b, "swagger") {
		return RoleContract, "schema or API definition"
	}
	if docExts[e] || firstSeg(p) == "docs" || strings.HasPrefix(b, "LICENSE") || strings.HasPrefix(b, "NOTICE") || strings.HasPrefix(b, "CHANGELOG") {
		return RoleDocs, "documentation"
	}
	if b == "go.mod" || configExts[e] || strings.HasPrefix(b, "Dockerfile") || configBaseNames[b] || strings.HasPrefix(p, ".github/") {
		return RoleConfig, "configuration"
	}
	return RoleOther, "other file"
}
