package target

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/sakshamsaxena/blaze/internal/config"
	"github.com/sakshamsaxena/blaze/internal/gitx"
)

// ghPR is the subset of `gh pr view --json ...` this package decodes.
type ghPR struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	URL    string `json:"url"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	BaseRefName string `json:"baseRefName"`
	BaseRefOid  string `json:"baseRefOid"`
	HeadRefName string `json:"headRefName"`
	HeadRefOid  string `json:"headRefOid"`
}

// ghFields are the JSON fields requested from `gh pr view`.
const ghFields = "number,title,body,url,author,baseRefName,baseRefOid,headRefName,headRefOid"

// runGH runs `gh <args...>` in dir and returns stdout. Tests replace it with
// a stub that returns canned JSON, or an error wrapping exec.ErrNotFound to
// simulate a missing gh binary.
var runGH = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// resolveGitHub resolves a Target from a GitHub pull request number or URL
// via the gh CLI.
func resolveGitHub(ctx context.Context, git *gitx.Runner, cfg config.Config) (Target, error) {
	root, err := git.RepoRoot(ctx)
	if err != nil {
		return Target{}, fmt.Errorf("resolve github target: %w", err)
	}

	out, err := runGH(ctx, root, "pr", "view", cfg.PR, "--json", ghFields)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return Target{}, fmt.Errorf("the gh CLI is required to review a PR by number; install it or use --base/--head")
		}
		return Target{}, fmt.Errorf("resolve github target: %w", err)
	}

	var pr ghPR
	if err := json.Unmarshal(out, &pr); err != nil {
		return Target{}, fmt.Errorf("resolve github target: decode gh output: %w", err)
	}

	if !git.HasCommit(ctx, pr.HeadRefOid) {
		if err := git.Fetch(ctx, cfg.Remote, fmt.Sprintf("refs/pull/%d/head", pr.Number)); err != nil {
			return Target{}, fmt.Errorf("resolve github target: %w", err)
		}
	}
	if !git.HasCommit(ctx, pr.BaseRefOid) {
		if err := git.Fetch(ctx, cfg.Remote, pr.BaseRefName); err != nil {
			return Target{}, fmt.Errorf("resolve github target: %w", err)
		}
	}
	if !git.HasCommit(ctx, pr.HeadRefOid) {
		return Target{}, fmt.Errorf("resolve github target: head commit %s not found after fetch", pr.HeadRefOid)
	}
	if !git.HasCommit(ctx, pr.BaseRefOid) {
		return Target{}, fmt.Errorf("resolve github target: base commit %s not found after fetch", pr.BaseRefOid)
	}

	baseSHA, err := git.MergeBase(ctx, pr.BaseRefOid, pr.HeadRefOid)
	if err != nil {
		return Target{}, fmt.Errorf("resolve github target: %w", err)
	}

	return Target{
		Kind:     KindGitHub,
		Title:    pr.Title,
		Body:     pr.Body,
		URL:      pr.URL,
		Number:   pr.Number,
		Author:   pr.Author.Login,
		BaseRef:  pr.BaseRefName,
		HeadRef:  pr.HeadRefName,
		BaseSHA:  baseSHA,
		HeadSHA:  pr.HeadRefOid,
		StateKey: fmt.Sprintf("pr-%d", pr.Number),
	}, nil
}
