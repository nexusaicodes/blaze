package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sakshamsaxena/blaze/internal/config"
	"github.com/sakshamsaxena/blaze/internal/logger"
	"github.com/sakshamsaxena/blaze/internal/plan"
	"github.com/sakshamsaxena/blaze/internal/testutil"
)

func TestMain(m *testing.M) {
	logger.InitNop()
	os.Exit(m.Run())
}

// TestFixturePlan runs the same pipeline as
// `blaze plan --repo <fixture> --base main --head feature`, in-process,
// over the 08-fixture.md fixture, and asserts the golden plan it documents.
func TestFixturePlan(t *testing.T) {
	fx := testutil.NewFixture(t)
	cfg := &config.Config{Repo: fx.Dir, Base: "main", Head: "feature", Remote: "origin", ModuleDir: "."}

	p, err := buildPlan(context.Background(), cfg, io.Discard)
	require.NoError(t, err)

	byTitle := make(map[string]*plan.Item, len(p.Items))
	for _, it := range p.Items {
		byTitle[it.Title] = it
	}
	require.Len(t, byTitle, len(p.Items), "item titles must be unique")

	// -- items (08-fixture.md "Expected plan / Items") --
	type wantItem struct {
		kind    plan.ItemKind
		status  plan.ChangeStatus
		role    plan.Role
		reason  string
		added   int
		deleted int
	}
	wantItems := map[string]wantItem{
		"store.Record":           {plan.ItemSymbol, plan.ChangeModified, plan.RoleContract, "type declaration", 1, 0},
		"store.Store":            {plan.ItemSymbol, plan.ChangeModified, plan.RoleContract, "type declaration", 1, 0},
		"store.MemStore.Save":    {plan.ItemSymbol, plan.ChangeModified, plan.RoleLogic, "implementation", 1, 0},
		"store.MemStore.Delete":  {plan.ItemSymbol, plan.ChangeAdded, plan.RoleLogic, "implementation", 6, 0},
		"service.Service.Remove": {plan.ItemSymbol, plan.ChangeAdded, plan.RoleLogic, "implementation", 5, 0},
		"store.TestDelete":       {plan.ItemSymbol, plan.ChangeAdded, plan.RoleTest, "test code", 7, 0},
		"README.md":              {plan.ItemFile, plan.ChangeModified, plan.RoleDocs, "documentation", 2, 0},
		"bin.dat":                {plan.ItemFile, plan.ChangeAdded, plan.RoleOther, "other file", 0, 0},
		"new name.txt":           {plan.ItemFile, plan.ChangeRenamed, plan.RoleMechanical, "renamed without changes", 0, 0},
	}
	require.Len(t, p.Items, len(wantItems))
	for title, want := range wantItems {
		it, ok := byTitle[title]
		require.True(t, ok, "missing item %q", title)
		require.Equal(t, want.kind, it.Kind, title)
		require.Equal(t, want.status, it.Status, title)
		require.Equal(t, want.role, it.Role, title)
		require.Equal(t, want.reason, it.RoleReason, title)
		require.Equal(t, want.added, it.Added, title)
		require.Equal(t, want.deleted, it.Deleted, title)
	}

	// -- threads (08-fixture.md "Expected plan / Threads"), in order --
	type wantThread struct {
		kind     plan.ThreadKind
		title    string
		subtitle string
		score    int
		level    string
	}
	wantThreads := []wantThread{
		{plan.ThreadCode, "store.Store + 5 more", "service, store", 50, "high"},
		{plan.ThreadDocs, "Documentation", "1 file", 0, "none"},
		{plan.ThreadOther, "Other files", "1 file", 0, "none"},
		{plan.ThreadMechanical, "Mechanical changes", "1 file", 0, "none"},
	}
	require.Len(t, p.Threads, len(wantThreads))
	for i, want := range wantThreads {
		th := p.Threads[i]
		require.Equal(t, want.kind, th.Kind, i)
		require.Equal(t, want.title, th.Title, i)
		require.Equal(t, want.subtitle, th.Subtitle, i)
		require.Equal(t, want.score, th.Risk.Score, i)
		require.Equal(t, want.level, th.Risk.Level, i)
	}

	// -- item order of thread 1 --
	wantOrder := []string{
		"store.Record",
		"store.Store",
		"store.MemStore.Delete",
		"service.Service.Remove",
		"store.MemStore.Save",
		"store.TestDelete",
	}
	thread1 := p.Threads[0]
	require.Len(t, thread1.ItemIDs, len(wantOrder))
	for i, id := range thread1.ItemIDs {
		require.Equal(t, wantOrder[i], p.Items[id].Title, "thread1 position %d", i)
	}

	// -- blast and risk (08-fixture.md "Expected plan / Blast and risk") --
	type wantBlast struct {
		outside, inPR, tests, changedTests, otherPkgs int
		score                                         int
		level                                         string
	}
	wantBlasts := map[string]wantBlast{
		"store.Record":           {5, 2, 1, 0, 1, 50, "high"},
		"store.Store":            {4, 1, 0, 0, 1, 50, "high"},
		"store.MemStore.Save":    {1, 0, 1, 0, 1, 35, "medium"},
		"store.MemStore.Delete":  {0, 1, 1, 1, 0, 0, "low"},
		"service.Service.Remove": {0, 0, 0, 0, 0, 15, "low"},
	}
	for title, want := range wantBlasts {
		it := byTitle[title]
		require.NotNil(t, it.Blast, title)
		require.Equal(t, want.outside, it.Blast.OutsideCount, title)
		require.Equal(t, want.inPR, it.Blast.InPRCount, title)
		require.Equal(t, want.tests, it.Blast.TestCount, title)
		require.Equal(t, want.changedTests, it.Blast.ChangedTests, title)
		require.Equal(t, want.otherPkgs, it.Blast.OtherPackages, title)
		require.Equal(t, want.score, it.Risk.Score, title)
		require.Equal(t, want.level, it.Risk.Level, title)
	}

	// -- stats --
	require.Equal(t, plan.Stats{Files: 6, Items: 9, Threads: 4, Added: 23, Deleted: 0, Unresolved: 0}, p.Stats)

	// -- determinism: running the pipeline twice yields identical JSON --
	p2, err := buildPlan(context.Background(), cfg, io.Discard)
	require.NoError(t, err)

	var b1, b2 bytes.Buffer
	require.NoError(t, writePlan(&b1, p))
	require.NoError(t, writePlan(&b2, p2))
	require.Equal(t, b1.Bytes(), b2.Bytes())
}
