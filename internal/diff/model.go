// Package diff parses git's name-status and unified-patch output into a
// structured per-file, per-hunk, per-line model.
package diff

// FileStatus is how a file changed between two commits.
type FileStatus string

const (
	StatusAdded       FileStatus = "added"
	StatusModified    FileStatus = "modified"
	StatusDeleted     FileStatus = "deleted"
	StatusRenamed     FileStatus = "renamed"
	StatusCopied      FileStatus = "copied"
	StatusTypeChanged FileStatus = "typechanged"
)

// LineKind is the role of one diff line.
type LineKind string

const (
	LineContext LineKind = "context"
	LineAdd     LineKind = "add"
	LineDel     LineKind = "del"
)

// Line is one line of a hunk.
type Line struct {
	Kind  LineKind `json:"kind"`
	OldNo int      `json:"oldNo"` // 0 for add
	NewNo int      `json:"newNo"` // 0 for del
	Text  string   `json:"text"`  // without the leading ' ', '+' or '-'; no trailing newline
	NoEOL bool     `json:"noEol,omitempty"`
}

// Hunk is one contiguous block of changed and context lines.
type Hunk struct {
	OldStart int    `json:"oldStart"`
	OldLines int    `json:"oldLines"`
	NewStart int    `json:"newStart"`
	NewLines int    `json:"newLines"`
	Section  string `json:"section"`
	Lines    []Line `json:"lines"`
}

// FileDiff is one changed file.
type FileDiff struct {
	OldPath string     `json:"oldPath"` // "" when added
	NewPath string     `json:"newPath"` // "" when deleted
	Status  FileStatus `json:"status"`
	Binary  bool       `json:"binary"`
	Hunks   []Hunk     `json:"hunks"`
	Added   int        `json:"added"`
	Deleted int        `json:"deleted"`
}

// Path returns NewPath, or OldPath for a deleted file.
func (f FileDiff) Path() string {
	if f.NewPath != "" {
		return f.NewPath
	}
	return f.OldPath
}
