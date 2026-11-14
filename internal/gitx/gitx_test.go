package gitx

import (
	"context"
	"strings"
	"testing"

	"github.com/sakshamsaxena/blaze/internal/testutil"
	"github.com/stretchr/testify/require"
)

// newTestRunner builds a Runner over the fixture repository with its
// worktree cache pointed at a temp directory.
func newTestRunner(t *testing.T, dir string) *Runner {
	t.Helper()
	r := New(dir)
	r.cacheDir = t.TempDir()
	return r
}

func TestRepoRoot(t *testing.T) {
	fx := testutil.NewFixture(t)
	r := newTestRunner(t, fx.Dir)

	root, err := r.RepoRoot(context.Background())
	require.NoError(t, err)
	require.Equal(t, fx.Dir, root)
}

func TestResolveCommit(t *testing.T) {
	fx := testutil.NewFixture(t)
	r := newTestRunner(t, fx.Dir)
	ctx := context.Background()

	sha, err := r.ResolveCommit(ctx, "feature")
	require.NoError(t, err)
	require.Equal(t, fx.HeadSHA, sha)

	_, err = r.ResolveCommit(ctx, "no-such-ref")
	require.Error(t, err)
}

func TestMergeBase(t *testing.T) {
	fx := testutil.NewFixture(t)
	r := newTestRunner(t, fx.Dir)

	sha, err := r.MergeBase(context.Background(), "main", "feature")
	require.NoError(t, err)
	require.Equal(t, fx.BaseSHA, sha)
}

func TestNameStatus(t *testing.T) {
	fx := testutil.NewFixture(t)
	r := newTestRunner(t, fx.Dir)

	out, err := r.NameStatus(context.Background(), fx.BaseSHA, fx.HeadSHA)
	require.NoError(t, err)

	want := "M\x00README.md\x00A\x00bin.dat\x00R100\x00old name.txt\x00new name.txt\x00" +
		"M\x00service/service.go\x00M\x00store/store.go\x00M\x00store/store_test.go\x00"
	require.Equal(t, want, string(out))
}

func TestShow(t *testing.T) {
	fx := testutil.NewFixture(t)
	r := newTestRunner(t, fx.Dir)

	out, err := r.Show(context.Background(), fx.BaseSHA, "store/store.go")
	require.NoError(t, err)
	require.Contains(t, string(out), "package store")
	require.NotContains(t, string(out), "Delete")
}

func TestExists(t *testing.T) {
	fx := testutil.NewFixture(t)
	r := newTestRunner(t, fx.Dir)
	ctx := context.Background()

	require.True(t, r.Exists(ctx, fx.HeadSHA, "store/store.go"))
	require.False(t, r.Exists(ctx, fx.HeadSHA, "no/such/path.go"))
}

func TestEnsureWorktree(t *testing.T) {
	fx := testutil.NewFixture(t)
	r := newTestRunner(t, fx.Dir)
	ctx := context.Background()

	path, err := r.EnsureWorktree(ctx, fx.HeadSHA)
	require.NoError(t, err)

	head, err := New(path).run(ctx, "rev-parse", "HEAD")
	require.NoError(t, err)
	require.Equal(t, fx.HeadSHA, strings.TrimSpace(string(head)))

	path2, err := r.EnsureWorktree(ctx, fx.HeadSHA)
	require.NoError(t, err)
	require.Equal(t, path, path2)
}
