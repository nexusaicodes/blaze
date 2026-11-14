// Command blaze is the CLI entry point: flag parsing and dependency wiring
// for the review and plan pipelines.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/sakshamsaxena/blaze/internal/config"
	"github.com/sakshamsaxena/blaze/internal/logger"
	"github.com/spf13/cobra"
)

// version is the printed build version. It has no release process yet, so
// it is a fixed string.
const version = "0.0.0-dev"

func main() {
	if err := newRootCmd().Execute(); err != nil {
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
// The pipeline is not implemented yet.
func run(ctx context.Context, cfg *config.Config, mode string) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := logger.Init(cfg.LogLevel); err != nil {
		return err
	}
	_ = mode
	return fmt.Errorf("not implemented")
}
