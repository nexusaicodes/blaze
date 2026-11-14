// Package target resolves what is being reviewed: a local ref range or a
// GitHub pull request.
package target

import (
	"context"

	"github.com/sakshamsaxena/blaze/internal/config"
	"github.com/sakshamsaxena/blaze/internal/gitx"
)

// Kind is what a Target was resolved from.
type Kind string

const (
	KindLocal  Kind = "local"
	KindGitHub Kind = "github"
)

// Target is what is being reviewed: the two commits to diff, plus PR
// metadata when the target came from GitHub.
type Target struct {
	Kind     Kind   `json:"kind"`
	RepoRoot string `json:"-"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	URL      string `json:"url"`     // "" for local
	Number   int    `json:"number"`  // 0 for local
	Author   string `json:"author"`  // "" for local
	BaseRef  string `json:"baseRef"` // name as given or as reported by gh
	HeadRef  string `json:"headRef"`
	BaseSHA  string `json:"baseSha"` // the MERGE BASE, not the base branch tip
	HeadSHA  string `json:"headSha"`
	StateKey string `json:"-"`
}

// Resolve resolves cfg into a Target: a GitHub PR when cfg.PR is set,
// otherwise a local ref range.
func Resolve(ctx context.Context, git *gitx.Runner, cfg config.Config) (Target, error) {
	if cfg.PR != "" {
		return resolveGitHub(ctx, git, cfg)
	}
	return resolveLocal(ctx, git, cfg)
}
