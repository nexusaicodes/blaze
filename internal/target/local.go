package target

import (
	"context"
	"fmt"
	"regexp"

	"github.com/sakshamsaxena/blaze/internal/config"
	"github.com/sakshamsaxena/blaze/internal/gitx"
)

// sanitizeRe matches characters not allowed in a StateKey.
var sanitizeRe = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// sanitize replaces every character outside [A-Za-z0-9._-] with '_'.
func sanitize(s string) string {
	return sanitizeRe.ReplaceAllString(s, "_")
}

// resolveLocal resolves a Target from a base/head ref pair.
func resolveLocal(ctx context.Context, git *gitx.Runner, cfg config.Config) (Target, error) {
	baseRef := cfg.Base
	if baseRef == "" {
		var err error
		baseRef, err = git.DefaultBase(ctx, cfg.Remote)
		if err != nil {
			return Target{}, fmt.Errorf("resolve local target: %w", err)
		}
	}

	headSHA, err := git.ResolveCommit(ctx, cfg.Head)
	if err != nil {
		return Target{}, fmt.Errorf("resolve local target: %w", err)
	}
	baseTip, err := git.ResolveCommit(ctx, baseRef)
	if err != nil {
		return Target{}, fmt.Errorf("resolve local target: %w", err)
	}
	baseSHA, err := git.MergeBase(ctx, baseTip, headSHA)
	if err != nil {
		return Target{}, fmt.Errorf("resolve local target: %w", err)
	}
	if baseSHA == headSHA {
		return Target{}, fmt.Errorf("nothing to review: %s has no commits beyond %s", cfg.Head, baseRef)
	}

	return Target{
		Kind:     KindLocal,
		Title:    fmt.Sprintf("%s → %s", cfg.Head, baseRef),
		BaseRef:  baseRef,
		HeadRef:  cfg.Head,
		BaseSHA:  baseSHA,
		HeadSHA:  headSHA,
		StateKey: "local-" + sanitize(baseRef) + "--" + sanitize(cfg.Head),
	}, nil
}
