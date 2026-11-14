package target

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"testing"

	"github.com/sakshamsaxena/blaze/internal/config"
	"github.com/sakshamsaxena/blaze/internal/gitx"
	"github.com/sakshamsaxena/blaze/internal/testutil"
	"github.com/stretchr/testify/require"
)

// stubRunGH replaces runGH for the duration of the test and restores it on
// cleanup.
func stubRunGH(t *testing.T, fn func(ctx context.Context, dir string, args ...string) ([]byte, error)) {
	t.Helper()
	orig := runGH
	runGH = fn
	t.Cleanup(func() { runGH = orig })
}

func TestResolveGitHub(t *testing.T) {
	fx := testutil.NewFixture(t)
	git := gitx.New(fx.Dir)

	pr := ghPR{
		Number:      482,
		Title:       "Add feature",
		Body:        "a description",
		URL:         "https://github.com/example/fixture/pull/482",
		BaseRefName: "main",
		BaseRefOid:  fx.BaseSHA,
		HeadRefName: "feature",
		HeadRefOid:  fx.HeadSHA,
	}
	pr.Author.Login = "octocat"
	canned, err := json.Marshal(pr)
	require.NoError(t, err)

	stubRunGH(t, func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return canned, nil
	})

	cfg := config.Config{PR: "482", Remote: "origin"}
	tgt, err := Resolve(context.Background(), git, cfg)
	require.NoError(t, err)

	require.Equal(t, KindGitHub, tgt.Kind)
	require.Equal(t, "Add feature", tgt.Title)
	require.Equal(t, 482, tgt.Number)
	require.Equal(t, fx.BaseSHA, tgt.BaseSHA)
	require.Equal(t, fx.HeadSHA, tgt.HeadSHA)
	require.Equal(t, "pr-482", tgt.StateKey)
}

func TestResolveGitHubMissingGH(t *testing.T) {
	fx := testutil.NewFixture(t)
	git := gitx.New(fx.Dir)

	stubRunGH(t, func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("exec: %w", &exec.Error{Name: "gh", Err: exec.ErrNotFound})
	})

	cfg := config.Config{PR: "482", Remote: "origin"}
	_, err := Resolve(context.Background(), git, cfg)
	require.EqualError(t, err, "the gh CLI is required to review a PR by number; install it or use --base/--head")
}
