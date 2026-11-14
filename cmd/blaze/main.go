// Command blaze is the CLI entry point: flag parsing and dependency wiring
// for the review and plan pipelines.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/sakshamsaxena/blaze/internal/config"
	"github.com/sakshamsaxena/blaze/internal/diff"
	"github.com/sakshamsaxena/blaze/internal/dodge"
	"github.com/sakshamsaxena/blaze/internal/gitx"
	"github.com/sakshamsaxena/blaze/internal/logger"
	"github.com/sakshamsaxena/blaze/internal/plan"
	"github.com/sakshamsaxena/blaze/internal/target"
	"github.com/spf13/cobra"
)

// version is the printed build version. It has no release process yet, so
// it is a fixed string.
const version = "0.0.0-dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := newRootCmd().ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "blaze",
		Short:         "Blaze reorders a PR into review order and shows blast radius",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(newReviewCmd(), newPlanCmd(), newVersionCmd())
	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the blaze version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), version)
			return nil
		},
	}
}

func newReviewCmd() *cobra.Command {
	cfg := &config.Config{}
	cmd := &cobra.Command{
		Use:   "review [<pr>] [flags]",
		Short: "Build the plan, serve the UI, open the browser",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				cfg.PR = args[0]
			}
			return run(cmd.Context(), cfg, "review")
		},
	}
	addCommonFlags(cmd, cfg)
	addReviewFlags(cmd, cfg)
	return cmd
}

func newPlanCmd() *cobra.Command {
	cfg := &config.Config{}
	cmd := &cobra.Command{
		Use:   "plan [<pr>] [flags]",
		Short: "Build the plan, print JSON to stdout, exit 0",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				cfg.PR = args[0]
			}
			return run(cmd.Context(), cfg, "plan")
		},
	}
	addCommonFlags(cmd, cfg)
	return cmd
}

// addCommonFlags registers the flags shared by `review` and `plan`.
func addCommonFlags(cmd *cobra.Command, cfg *config.Config) {
	cmd.Flags().StringVar(&cfg.Repo, "repo", ".", "Any directory inside the repository")
	cmd.Flags().StringVar(&cfg.Base, "base", "", "Base ref for a local target")
	cmd.Flags().StringVar(&cfg.Head, "head", "HEAD", "Head ref for a local target")
	cmd.Flags().StringVar(&cfg.Remote, "remote", "origin", "Remote used to fetch PR refs")
	cmd.Flags().StringVar(&cfg.ModuleDir, "module-dir", ".", "Directory of go.mod, relative to the repo root")
	cmd.Flags().StringVar(&cfg.LogLevel, "log-level", "warn", "debug, info, warn, error")
}

// addReviewFlags registers the flags specific to `review`.
func addReviewFlags(cmd *cobra.Command, cfg *config.Config) {
	cmd.Flags().IntVar(&cfg.Port, "port", 0, "TCP port on 127.0.0.1; 0 picks a free port")
	cmd.Flags().BoolVar(&cfg.NoOpen, "no-open", false, "Do not open the browser")
	cmd.Flags().BoolVar(&cfg.NoState, "no-state", false, "Keep review marks in memory only")
}

// run validates cfg and executes the pipeline for mode "review" or "plan".
// Mode "review" is not implemented yet.
func run(ctx context.Context, cfg *config.Config, mode string) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := logger.Init(cfg.LogLevel); err != nil {
		return err
	}

	switch mode {
	case "plan":
		p, err := buildPlan(ctx, cfg, os.Stderr)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "blaze: plan ready: %d threads, %d items\n", p.Stats.Threads, p.Stats.Items)
		return writePlan(os.Stdout, p)
	case "review":
		return fmt.Errorf("review mode: state and server are not implemented yet")
	default:
		return fmt.Errorf("unknown mode %q", mode)
	}
}

// buildPlan runs the pipeline common to both modes: resolve the target,
// build the head symbol graph, parse the diff, and build the Plan.
// Progress lines go to stderr.
func buildPlan(ctx context.Context, cfg *config.Config, stderr io.Writer) (*plan.Plan, error) {
	git := gitx.New(cfg.Repo)
	root, err := git.RepoRoot(ctx)
	if err != nil {
		return nil, fmt.Errorf("repo root: %w", err)
	}
	git = git.WithDir(root)

	fmt.Fprintln(stderr, "blaze: resolving target …")
	tgt, err := target.Resolve(ctx, git, *cfg)
	if err != nil {
		return nil, fmt.Errorf("resolve target: %w", err)
	}

	wt, err := git.EnsureWorktree(ctx, tgt.HeadSHA)
	if err != nil {
		return nil, fmt.Errorf("ensure worktree: %w", err)
	}

	fmt.Fprintln(stderr, "blaze: analysing packages …")
	gp, err := dodge.New("go")
	if err != nil {
		return nil, fmt.Errorf("dodge provider: %w", err)
	}
	graph, err := gp.Build(ctx, dodge.BuildInput{RepoRoot: wt, ModuleDir: cfg.ModuleDir})
	if err != nil {
		return nil, fmt.Errorf("build graph: %w", err)
	}

	raw, err := git.Diff(ctx, tgt.BaseSHA, tgt.HeadSHA)
	if err != nil {
		return nil, fmt.Errorf("diff: %w", err)
	}
	names, err := git.NameStatus(ctx, tgt.BaseSHA, tgt.HeadSHA)
	if err != nil {
		return nil, fmt.Errorf("name status: %w", err)
	}
	files, err := diff.Parse(names, raw)
	if err != nil {
		return nil, fmt.Errorf("parse diff: %w", err)
	}

	p, err := plan.Build(ctx, plan.Input{
		Target:    tgt,
		Files:     files,
		Graph:     graph,
		Provider:  gp,
		Git:       git,
		Worktree:  wt,
		ModuleDir: cfg.ModuleDir,
	})
	if err != nil {
		return nil, fmt.Errorf("build plan: %w", err)
	}
	return p, nil
}

// writePlan encodes p as indented JSON to w, exactly as mode "plan" writes
// it to stdout.
func writePlan(w io.Writer, p *plan.Plan) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(p)
}
