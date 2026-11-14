// Package gitx wraps the git CLI. Every command runs through
// exec.CommandContext with an argument slice, never a shell.
package gitx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Runner runs git commands against one working directory.
type Runner struct {
	dir      string // working directory for every command
	cacheDir string // root for worktrees; defaults to os.UserCacheDir(), overridden in tests
}

// New builds a Runner that runs git commands in dir.
func New(dir string) *Runner {
	cacheDir, _ := os.UserCacheDir()
	return &Runner{dir: dir, cacheDir: cacheDir}
}

// WithDir returns a copy of r that runs commands in dir instead.
func (r *Runner) WithDir(dir string) *Runner {
	c := *r
	c.dir = dir
	return &c
}

// run executes `git <args...>` in r.dir and returns stdout. On a non-zero
// exit it returns an error containing the arguments and trimmed stderr.
func (r *Runner) run(ctx context.Context, args ...string) ([]byte, error) {
	full := append([]string{"-c", "core.quotepath=false", "-c", "color.ui=false"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Dir = r.dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// RepoRoot returns the absolute, trimmed path of the repository's top
// level.
func (r *Runner) RepoRoot(ctx context.Context) (string, error) {
	out, err := r.run(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("repo root: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ResolveCommit resolves ref to a 40-character commit SHA. It errors if ref
// does not resolve to a commit.
func (r *Runner) ResolveCommit(ctx context.Context, ref string) (string, error) {
	out, err := r.run(ctx, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("resolve commit %q: %w", ref, err)
	}
	sha := strings.TrimSpace(string(out))
	if sha == "" {
		return "", fmt.Errorf("resolve commit %q: no such commit", ref)
	}
	return sha, nil
}

// MergeBase returns the merge base of a and b.
func (r *Runner) MergeBase(ctx context.Context, a, b string) (string, error) {
	out, err := r.run(ctx, "merge-base", a, b)
	if err != nil {
		return "", fmt.Errorf("merge base %s %s: %w", a, b, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// DefaultBase guesses the base branch for remote, trying in order:
// `<remote>/HEAD`'s symbolic ref, `<remote>/main`, `<remote>/master`,
// `main`, `master`. It returns the first ref name that ResolveCommit
// accepts.
func (r *Runner) DefaultBase(ctx context.Context, remote string) (string, error) {
	if out, err := r.run(ctx, "symbolic-ref", "--quiet", "--short", "refs/remotes/"+remote+"/HEAD"); err == nil {
		if ref := strings.TrimSpace(string(out)); ref != "" {
			if _, err := r.ResolveCommit(ctx, ref); err == nil {
				return ref, nil
			}
		}
	}

	candidates := []string{remote + "/main", remote + "/master", "main", "master"}
	for _, ref := range candidates {
		if _, err := r.ResolveCommit(ctx, ref); err == nil {
			return ref, nil
		}
	}
	return "", fmt.Errorf("cannot determine base branch; pass --base")
}

// HasCommit reports whether sha exists as a commit.
func (r *Runner) HasCommit(ctx context.Context, sha string) bool {
	_, err := r.run(ctx, "cat-file", "-e", sha+"^{commit}")
	return err == nil
}

// Fetch fetches refspec from remote.
func (r *Runner) Fetch(ctx context.Context, remote, refspec string) error {
	if _, err := r.run(ctx, "fetch", "--quiet", "--no-tags", remote, refspec); err != nil {
		return fmt.Errorf("fetch %s %s: %w", remote, refspec, err)
	}
	return nil
}

// NameStatus returns the raw NUL-separated name-status output between base
// and head.
func (r *Runner) NameStatus(ctx context.Context, base, head string) ([]byte, error) {
	out, err := r.run(ctx, "diff", "--name-status", "-z", "-M", base, head)
	if err != nil {
		return nil, fmt.Errorf("name status %s %s: %w", base, head, err)
	}
	return out, nil
}

// Diff returns the raw unified patch between base and head.
func (r *Runner) Diff(ctx context.Context, base, head string) ([]byte, error) {
	out, err := r.run(ctx, "diff", "--no-color", "--no-ext-diff", "-M", "-U3", "--src-prefix=a/", "--dst-prefix=b/", base, head)
	if err != nil {
		return nil, fmt.Errorf("diff %s %s: %w", base, head, err)
	}
	return out, nil
}

// Show returns the bytes of path as it exists at sha.
func (r *Runner) Show(ctx context.Context, sha, path string) ([]byte, error) {
	out, err := r.run(ctx, "show", sha+":"+path)
	if err != nil {
		return nil, fmt.Errorf("show %s:%s: %w", sha, path, err)
	}
	return out, nil
}

// Exists reports whether path exists at sha.
func (r *Runner) Exists(ctx context.Context, sha, path string) bool {
	_, err := r.run(ctx, "cat-file", "-e", sha+":"+path)
	return err == nil
}

// EnsureWorktree returns the path of a detached checkout of sha, creating
// or reusing one under r.cacheDir. Worktrees are kept between calls.
func (r *Runner) EnsureWorktree(ctx context.Context, sha string) (string, error) {
	root, err := r.RepoRoot(ctx)
	if err != nil {
		return "", fmt.Errorf("ensure worktree: %w", err)
	}
	sum := sha256.Sum256([]byte(root))
	repoKey := hex.EncodeToString(sum[:])[:12]
	path := filepath.Join(r.cacheDir, "blaze", "worktrees", repoKey, sha[:12])

	if _, err := os.Stat(path); err == nil {
		if out, err := r.WithDir(path).run(ctx, "rev-parse", "HEAD"); err == nil && strings.TrimSpace(string(out)) == sha {
			return path, nil
		}
		_, _ = r.run(ctx, "worktree", "remove", "--force", path)
		if err := os.RemoveAll(path); err != nil {
			return "", fmt.Errorf("ensure worktree: remove %s: %w", path, err)
		}
		if _, err := r.run(ctx, "worktree", "prune"); err != nil {
			return "", fmt.Errorf("ensure worktree: prune: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("ensure worktree: mkdir %s: %w", filepath.Dir(path), err)
	}
	if _, err := r.run(ctx, "worktree", "add", "--detach", "--quiet", path, sha); err != nil {
		return "", fmt.Errorf("ensure worktree: add %s: %w", sha, err)
	}
	return path, nil
}
