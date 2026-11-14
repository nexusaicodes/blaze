package plan

import (
	"context"
	"fmt"
	"sort"

	"github.com/sakshamsaxena/blaze/internal/diff"
	"github.com/sakshamsaxena/blaze/internal/dodge"
	"github.com/sakshamsaxena/blaze/internal/gitx"
	"github.com/sakshamsaxena/blaze/internal/target"
)

// Input is the input to Build.
type Input struct {
	Target    target.Target
	Files     []diff.FileDiff
	Graph     *dodge.Graph         // head
	Provider  dodge.IGraphProvider // for syntax-only parsing
	Git       *gitx.Runner         // to read base files
	Worktree  string               // head checkout, to read source lines
	ModuleDir string
}

// Build runs the plan pipeline of 05-plan.md over in and returns the
// resulting Plan.
//
// This milestone runs steps 1-4 and 7 (attribute, classify, link, thread,
// order); Risk and Blast are left at their zero values until the steps
// that compute them.
func Build(ctx context.Context, in Input) (*Plan, error) {
	byKey, warnings, err := attribute(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("attribute: %w", err)
	}

	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	items := make(map[string]*Item, len(byKey))
	var added, deleted, unresolved int
	for _, k := range keys {
		item := byKey[k]
		item.Role, item.RoleReason = classify(item)
		item.Slices = nonNil(item.Slices)
		item.Uses = []Link{}
		item.UsedBy = []Link{}
		item.Risk = Risk{Level: "none", Reasons: []string{}}

		items[item.ID] = item
		added += item.Added
		deleted += item.Deleted
		if item.Kind == ItemSymbol && !item.Symbol.Resolved {
			unresolved++
		}
	}

	link(items, in.Graph)
	groups := buildThreads(items)
	threads := order(groups)
	for _, t := range threads {
		for _, id := range t.ItemIDs {
			items[id].ThreadID = t.ID
		}
	}

	files := buildFileSummaries(in.Files, byKey)

	allWarnings := append([]string{}, in.Graph.Warnings...)
	allWarnings = append(allWarnings, warnings...)
	allWarnings = nonNil(allWarnings)

	return &Plan{
		Version: 1,
		Target:  in.Target,
		Stats: Stats{
			Files:      len(files),
			Items:      len(items),
			Threads:    len(threads),
			Added:      added,
			Deleted:    deleted,
			Unresolved: unresolved,
		},
		Threads:  threads,
		Items:    items,
		Files:    files,
		Warnings: allWarnings,
	}, nil
}

// buildFileSummaries returns one FileSummary per entry of files, sorted by
// Path, listing the IDs of the items that have a slice in that file (for
// pure renames and binary files, the file item).
func buildFileSummaries(files []diff.FileDiff, byKey map[string]*Item) []FileSummary {
	summaries := make([]FileSummary, 0, len(files))
	for _, fd := range files {
		fs := FileSummary{
			Path:    fd.Path(),
			Status:  fd.Status,
			Binary:  fd.Binary,
			Added:   fd.Added,
			Deleted: fd.Deleted,
			ItemIDs: nonNil(itemIDsForFile(fd, byKey)),
		}
		if fs.Status == diff.StatusRenamed || fs.Status == diff.StatusCopied {
			fs.OldPath = fd.OldPath
		}
		summaries = append(summaries, fs)
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Path < summaries[j].Path })
	return summaries
}

// itemIDsForFile returns the IDs of the items with a slice whose Path is
// fd.Path(), plus the file item itself (present for a pure rename or a
// binary file, which have no slices), sorted by the items' Key.
func itemIDsForFile(fd diff.FileDiff, byKey map[string]*Item) []string {
	var keys []string
	seen := map[string]bool{}
	add := func(k string) {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}

	if _, ok := byKey["file:"+fd.Path()]; ok {
		add("file:" + fd.Path())
	}
	for k, item := range byKey {
		for _, sl := range item.Slices {
			if sl.Path == fd.Path() {
				add(k)
				break
			}
		}
	}

	sort.Strings(keys)
	ids := make([]string, len(keys))
	for i, k := range keys {
		ids[i] = byKey[k].ID
	}
	return ids
}

// nonNil returns s, or an empty, non-nil slice when s is nil, so JSON
// encodes an empty list rather than null.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
