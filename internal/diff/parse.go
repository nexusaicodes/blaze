package diff

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// hunkHeaderRe matches a hunk header such as "@@ -1,3 +1,4 @@ func foo() {".
var hunkHeaderRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@ ?(.*)$`)

// Parse joins git's NUL-separated name-status output with its unified
// patch, by position, into one FileDiff per entry. Paths come only from
// nameStatus; hunks come only from patch.
func Parse(nameStatus, patch []byte) ([]FileDiff, error) {
	entries, err := parseNameStatus(nameStatus)
	if err != nil {
		return nil, fmt.Errorf("diff: %w", err)
	}

	sections := splitSections(patch)
	if len(sections) != len(entries) {
		return nil, fmt.Errorf("diff: %d name-status entries but %d patch sections", len(entries), len(sections))
	}

	files := make([]FileDiff, len(entries))
	for i, entry := range entries {
		fd := entry
		if err := parseSection(sections[i], &fd); err != nil {
			return nil, fmt.Errorf("diff: %w", err)
		}
		files[i] = fd
	}
	return files, nil
}

// parseNameStatus reads the NUL-separated fields of `git diff --name-status
// -z -M` into one FileDiff (paths and status only) per entry.
func parseNameStatus(raw []byte) ([]FileDiff, error) {
	fields := strings.Split(string(raw), "\x00")
	if len(fields) > 0 && fields[len(fields)-1] == "" {
		fields = fields[:len(fields)-1]
	}

	var entries []FileDiff
	for i := 0; i < len(fields); {
		status := fields[i]
		i++
		if status == "" {
			return nil, fmt.Errorf("empty status field")
		}
		switch status[0] {
		case 'A':
			path := fields[i]
			i++
			entries = append(entries, FileDiff{NewPath: path, Status: StatusAdded})
		case 'M':
			path := fields[i]
			i++
			entries = append(entries, FileDiff{OldPath: path, NewPath: path, Status: StatusModified})
		case 'D':
			path := fields[i]
			i++
			entries = append(entries, FileDiff{OldPath: path, Status: StatusDeleted})
		case 'T':
			path := fields[i]
			i++
			entries = append(entries, FileDiff{OldPath: path, NewPath: path, Status: StatusTypeChanged})
		case 'R':
			oldPath, newPath := fields[i], fields[i+1]
			i += 2
			entries = append(entries, FileDiff{OldPath: oldPath, NewPath: newPath, Status: StatusRenamed})
		case 'C':
			oldPath, newPath := fields[i], fields[i+1]
			i += 2
			entries = append(entries, FileDiff{OldPath: oldPath, NewPath: newPath, Status: StatusCopied})
		default:
			return nil, fmt.Errorf("unknown status %q", status)
		}
	}
	return entries, nil
}

// splitSections splits patch into per-file sections. A section starts at
// each line beginning with "diff --git " and runs to the next such line or
// the end. Lines are split on '\n' only; a trailing '\r' stays in the line.
func splitSections(patch []byte) [][]string {
	if len(patch) == 0 {
		return nil
	}
	lines := strings.Split(string(patch), "\n")

	var sections [][]string
	var current []string
	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			if current != nil {
				sections = append(sections, current)
			}
			current = []string{line}
			continue
		}
		if current != nil {
			current = append(current, line)
		}
	}
	if current != nil {
		sections = append(sections, current)
	}
	return sections
}

// parseSection walks the lines of one file's section (after its "diff
// --git" line) and fills in fd.Binary, fd.Hunks, fd.Added and fd.Deleted.
func parseSection(lines []string, fd *FileDiff) error {
	var hunk *Hunk
	var oldNo, newNo int
	var oldRemaining, newRemaining int

	for _, line := range lines[1:] {
		if strings.HasPrefix(line, "\\") {
			if hunk != nil && len(hunk.Lines) > 0 {
				hunk.Lines[len(hunk.Lines)-1].NoEOL = true
			}
			continue
		}

		if m := hunkHeaderRe.FindStringSubmatch(line); m != nil {
			oldStart, err := strconv.Atoi(m[1])
			if err != nil {
				return fmt.Errorf("hunk header %q: %w", line, err)
			}
			oldLines := 1
			if m[2] != "" {
				if oldLines, err = strconv.Atoi(m[2]); err != nil {
					return fmt.Errorf("hunk header %q: %w", line, err)
				}
			}
			newStart, err := strconv.Atoi(m[3])
			if err != nil {
				return fmt.Errorf("hunk header %q: %w", line, err)
			}
			newLines := 1
			if m[4] != "" {
				if newLines, err = strconv.Atoi(m[4]); err != nil {
					return fmt.Errorf("hunk header %q: %w", line, err)
				}
			}

			fd.Hunks = append(fd.Hunks, Hunk{
				OldStart: oldStart,
				OldLines: oldLines,
				NewStart: newStart,
				NewLines: newLines,
				Section:  m[5],
			})
			hunk = &fd.Hunks[len(fd.Hunks)-1]
			oldNo, newNo = oldStart, newStart
			oldRemaining, newRemaining = oldLines, newLines
			continue
		}

		if hunk == nil || (oldRemaining == 0 && newRemaining == 0) {
			if strings.HasPrefix(line, "Binary files ") || line == "GIT binary patch" {
				fd.Binary = true
			}
			continue
		}

		if line == "" {
			hunk.Lines = append(hunk.Lines, Line{Kind: LineContext, OldNo: oldNo, NewNo: newNo})
			oldNo++
			newNo++
			oldRemaining--
			newRemaining--
			continue
		}

		text := line[1:]
		switch line[0] {
		case ' ':
			hunk.Lines = append(hunk.Lines, Line{Kind: LineContext, OldNo: oldNo, NewNo: newNo, Text: text})
			oldNo++
			newNo++
			oldRemaining--
			newRemaining--
		case '+':
			hunk.Lines = append(hunk.Lines, Line{Kind: LineAdd, NewNo: newNo, Text: text})
			fd.Added++
			newNo++
			newRemaining--
		case '-':
			hunk.Lines = append(hunk.Lines, Line{Kind: LineDel, OldNo: oldNo, Text: text})
			fd.Deleted++
			oldNo++
			oldRemaining--
		default:
			return fmt.Errorf("unexpected line %q in hunk", line)
		}
	}
	return nil
}
