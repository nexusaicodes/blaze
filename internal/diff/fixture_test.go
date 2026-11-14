package diff_test

import (
	"context"
	"testing"

	"github.com/sakshamsaxena/blaze/internal/diff"
	"github.com/sakshamsaxena/blaze/internal/gitx"
	"github.com/sakshamsaxena/blaze/internal/testutil"
	"github.com/stretchr/testify/require"
)

// TestParseFixture parses the fixture's real git output and checks the six
// files, their statuses and hunk counts against 08-fixture.md.
func TestParseFixture(t *testing.T) {
	fx := testutil.NewFixture(t)
	git := gitx.New(fx.Dir)
	ctx := context.Background()

	names, err := git.NameStatus(ctx, fx.BaseSHA, fx.HeadSHA)
	require.NoError(t, err)
	patch, err := git.Diff(ctx, fx.BaseSHA, fx.HeadSHA)
	require.NoError(t, err)

	files, err := diff.Parse(names, patch)
	require.NoError(t, err)

	want := []struct {
		path   string
		status diff.FileStatus
		hunks  int
	}{
		{"README.md", diff.StatusModified, 1},
		{"bin.dat", diff.StatusAdded, 0},
		{"new name.txt", diff.StatusRenamed, 0},
		{"service/service.go", diff.StatusModified, 1},
		{"store/store.go", diff.StatusModified, 3},
		{"store/store_test.go", diff.StatusModified, 1},
	}
	require.Len(t, files, len(want))
	for i, w := range want {
		require.Equal(t, w.path, files[i].Path(), "file %d path", i)
		require.Equal(t, w.status, files[i].Status, "file %d status", i)
		require.Len(t, files[i].Hunks, w.hunks, "file %d hunks", i)
	}
	require.True(t, files[1].Binary, "bin.dat should be marked binary")
}
