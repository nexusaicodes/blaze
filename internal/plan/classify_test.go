package plan

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sakshamsaxena/blaze/internal/dodge"
)

// TestClassify covers one case per row of the classification table in
// 05-plan.md.
func TestClassify(t *testing.T) {
	cases := []struct {
		name   string
		item   *Item
		role   Role
		reason string
	}{
		{
			name:   "1 symbol generated",
			item:   &Item{Kind: ItemSymbol, Symbol: &dodge.Symbol{Kind: dodge.KindFunc, Generated: true}},
			role:   RoleGenerated,
			reason: "generated file",
		},
		{
			name:   "1 preamble generated",
			item:   &Item{Kind: ItemPreamble, fileGenerated: true},
			role:   RoleGenerated,
			reason: "generated file",
		},
		{
			name:   "2 file vendored (vendor segment)",
			item:   &Item{Kind: ItemFile, Path: "vendor/lib/x.go"},
			role:   RoleGenerated,
			reason: "vendored dependency",
		},
		{
			name:   "2 file vendored (node_modules segment)",
			item:   &Item{Kind: ItemFile, Path: "web/node_modules/foo.js"},
			role:   RoleGenerated,
			reason: "vendored dependency",
		},
		{
			name:   "3 file lockfile",
			item:   &Item{Kind: ItemFile, Path: "go.sum"},
			role:   RoleGenerated,
			reason: "lockfile",
		},
		{
			name:   "4 file generated glob",
			item:   &Item{Kind: ItemFile, Path: "api/models_gen.go"},
			role:   RoleGenerated,
			reason: "generated file",
		},
		{
			name:   "5 preamble always",
			item:   &Item{Kind: ItemPreamble},
			role:   RoleMechanical,
			reason: "imports and file header",
		},
		{
			name:   "6 symbol format-only",
			item:   &Item{Kind: ItemSymbol, Symbol: &dodge.Symbol{Kind: dodge.KindFunc}, formatOnly: true},
			role:   RoleMechanical,
			reason: "comments or formatting only",
		},
		{
			name:   "7 file renamed without changes",
			item:   &Item{Kind: ItemFile, Status: ChangeRenamed, Binary: false},
			role:   RoleMechanical,
			reason: "renamed without changes",
		},
		{
			name:   "8 symbol test code",
			item:   &Item{Kind: ItemSymbol, Symbol: &dodge.Symbol{Kind: dodge.KindFunc, IsTest: true}},
			role:   RoleTest,
			reason: "test code",
		},
		{
			name:   "9 file testdata segment",
			item:   &Item{Kind: ItemFile, Path: "store/testdata/input.json"},
			role:   RoleTest,
			reason: "test file or test data",
		},
		{
			name:   "9 file test glob",
			item:   &Item{Kind: ItemFile, Path: "web/app.spec.ts"},
			role:   RoleTest,
			reason: "test file or test data",
		},
		{
			name:   "10 symbol type declaration",
			item:   &Item{Kind: ItemSymbol, Symbol: &dodge.Symbol{Kind: dodge.KindType}},
			role:   RoleContract,
			reason: "type declaration",
		},
		{
			name:   "11 symbol signature changed",
			item:   &Item{Kind: ItemSymbol, Symbol: &dodge.Symbol{Kind: dodge.KindFunc}, SignatureChanged: true},
			role:   RoleContract,
			reason: "signature changed",
		},
		{
			name:   "12 symbol exported symbol removed",
			item:   &Item{Kind: ItemSymbol, Status: ChangeDeleted, Symbol: &dodge.Symbol{Kind: dodge.KindFunc, Exported: true}},
			role:   RoleContract,
			reason: "exported symbol removed",
		},
		{
			name:   "13 symbol exported value",
			item:   &Item{Kind: ItemSymbol, Symbol: &dodge.Symbol{Kind: dodge.KindVar, Exported: true}},
			role:   RoleContract,
			reason: "exported value",
		},
		{
			name:   "14 symbol wiring (main package)",
			item:   &Item{Kind: ItemSymbol, Symbol: &dodge.Symbol{Kind: dodge.KindFunc, PkgName: "main", Name: "run", File: "cmd/app/main.go"}},
			role:   RoleWiring,
			reason: "entry point or wiring",
		},
		{
			name:   "14 symbol wiring (cmd segment)",
			item:   &Item{Kind: ItemSymbol, Symbol: &dodge.Symbol{Kind: dodge.KindFunc, PkgName: "tools", Name: "run", File: "cmd/tool/run.go"}},
			role:   RoleWiring,
			reason: "entry point or wiring",
		},
		{
			name:   "14 symbol wiring (name main)",
			item:   &Item{Kind: ItemSymbol, Symbol: &dodge.Symbol{Kind: dodge.KindFunc, PkgName: "somepkg", Name: "main", File: "somepkg/x.go"}},
			role:   RoleWiring,
			reason: "entry point or wiring",
		},
		{
			name:   "15 symbol otherwise",
			item:   &Item{Kind: ItemSymbol, Symbol: &dodge.Symbol{Kind: dodge.KindFunc, PkgName: "store", Name: "Save", File: "store/store.go"}},
			role:   RoleLogic,
			reason: "implementation",
		},
		{
			name:   "16 file schema extension",
			item:   &Item{Kind: ItemFile, Path: "api/schema.proto"},
			role:   RoleContract,
			reason: "schema or API definition",
		},
		{
			name:   "16 file migrations segment",
			item:   &Item{Kind: ItemFile, Path: "db/migrations/0001_init.up"},
			role:   RoleContract,
			reason: "schema or API definition",
		},
		{
			name:   "16 file openapi prefix (also matches a config extension)",
			item:   &Item{Kind: ItemFile, Path: "openapi.yaml"},
			role:   RoleContract,
			reason: "schema or API definition",
		},
		{
			name:   "17 file doc extension",
			item:   &Item{Kind: ItemFile, Path: "docs/guide.md"},
			role:   RoleDocs,
			reason: "documentation",
		},
		{
			name:   "17 file first segment docs",
			item:   &Item{Kind: ItemFile, Path: "docs/intro"},
			role:   RoleDocs,
			reason: "documentation",
		},
		{
			name:   "17 file LICENSE prefix",
			item:   &Item{Kind: ItemFile, Path: "LICENSE"},
			role:   RoleDocs,
			reason: "documentation",
		},
		{
			name:   "18 file go.mod",
			item:   &Item{Kind: ItemFile, Path: "go.mod"},
			role:   RoleConfig,
			reason: "configuration",
		},
		{
			name:   "18 file config extension",
			item:   &Item{Kind: ItemFile, Path: "config.yaml"},
			role:   RoleConfig,
			reason: "configuration",
		},
		{
			name:   "18 file Dockerfile prefix",
			item:   &Item{Kind: ItemFile, Path: "build/Dockerfile.prod"},
			role:   RoleConfig,
			reason: "configuration",
		},
		{
			name:   "18 file Makefile",
			item:   &Item{Kind: ItemFile, Path: "Makefile"},
			role:   RoleConfig,
			reason: "configuration",
		},
		{
			name:   "18 file .github path",
			item:   &Item{Kind: ItemFile, Path: ".github/workflows/ci.yml"},
			role:   RoleConfig,
			reason: "configuration",
		},
		{
			name:   "19 file otherwise",
			item:   &Item{Kind: ItemFile, Path: "assets/logo.png"},
			role:   RoleOther,
			reason: "other file",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			role, reason := classify(c.item)
			require.Equal(t, c.role, role)
			require.Equal(t, c.reason, reason)
		})
	}
}
