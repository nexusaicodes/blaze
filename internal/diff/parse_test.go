package diff_test

import (
	"testing"

	"github.com/sakshamsaxena/blaze/internal/diff"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name       string
		nameStatus string
		patch      string
		want       []diff.FileDiff
		wantErr    string
	}{
		{
			name:       "modified file with context add and del",
			nameStatus: "M\x00file.txt\x00",
			patch: "diff --git a/file.txt b/file.txt\n" +
				"index 1111111..2222222 100644\n" +
				"--- a/file.txt\n" +
				"+++ b/file.txt\n" +
				"@@ -1,3 +1,3 @@\n" +
				" line1\n" +
				"-line2\n" +
				"+line2 changed\n" +
				" line3\n",
			want: []diff.FileDiff{{
				OldPath: "file.txt", NewPath: "file.txt", Status: diff.StatusModified,
				Added: 1, Deleted: 1,
				Hunks: []diff.Hunk{{
					OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 3,
					Lines: []diff.Line{
						{Kind: diff.LineContext, OldNo: 1, NewNo: 1, Text: "line1"},
						{Kind: diff.LineDel, OldNo: 2, Text: "line2"},
						{Kind: diff.LineAdd, NewNo: 2, Text: "line2 changed"},
						{Kind: diff.LineContext, OldNo: 3, NewNo: 3, Text: "line3"},
					},
				}},
			}},
		},
		{
			name:       "hunk header with omitted counts",
			nameStatus: "M\x00a.txt\x00",
			patch: "diff --git a/a.txt b/a.txt\n" +
				"index 1111111..2222222 100644\n" +
				"--- a/a.txt\n" +
				"+++ b/a.txt\n" +
				"@@ -1 +1,3 @@\n" +
				"-old\n" +
				"+new1\n" +
				"+new2\n" +
				"+new3\n",
			want: []diff.FileDiff{{
				OldPath: "a.txt", NewPath: "a.txt", Status: diff.StatusModified,
				Added: 3, Deleted: 1,
				Hunks: []diff.Hunk{{
					OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 3,
					Lines: []diff.Line{
						{Kind: diff.LineDel, OldNo: 1, Text: "old"},
						{Kind: diff.LineAdd, NewNo: 1, Text: "new1"},
						{Kind: diff.LineAdd, NewNo: 2, Text: "new2"},
						{Kind: diff.LineAdd, NewNo: 3, Text: "new3"},
					},
				}},
			}},
		},
		{
			name:       "added file and deleted file",
			nameStatus: "A\x00added.txt\x00D\x00deleted.txt\x00",
			patch: "diff --git a/added.txt b/added.txt\n" +
				"new file mode 100644\n" +
				"index 0000000..1111111\n" +
				"--- /dev/null\n" +
				"+++ b/added.txt\n" +
				"@@ -0,0 +1,2 @@\n" +
				"+line1\n" +
				"+line2\n" +
				"diff --git a/deleted.txt b/deleted.txt\n" +
				"deleted file mode 100644\n" +
				"index 1111111..0000000\n" +
				"--- a/deleted.txt\n" +
				"+++ /dev/null\n" +
				"@@ -1,2 +0,0 @@\n" +
				"-line1\n" +
				"-line2\n",
			want: []diff.FileDiff{
				{
					NewPath: "added.txt", Status: diff.StatusAdded, Added: 2,
					Hunks: []diff.Hunk{{
						OldStart: 0, OldLines: 0, NewStart: 1, NewLines: 2,
						Lines: []diff.Line{
							{Kind: diff.LineAdd, NewNo: 1, Text: "line1"},
							{Kind: diff.LineAdd, NewNo: 2, Text: "line2"},
						},
					}},
				},
				{
					OldPath: "deleted.txt", Status: diff.StatusDeleted, Deleted: 2,
					Hunks: []diff.Hunk{{
						OldStart: 1, OldLines: 2, NewStart: 0, NewLines: 0,
						Lines: []diff.Line{
							{Kind: diff.LineDel, OldNo: 1, Text: "line1"},
							{Kind: diff.LineDel, OldNo: 2, Text: "line2"},
						},
					}},
				},
			},
		},
		{
			name:       "pure rename with a space in both names, no hunks",
			nameStatus: "R100\x00old name.txt\x00new name.txt\x00",
			patch: "diff --git a/old name.txt b/new name.txt\n" +
				"similarity index 100%\n" +
				"rename from old name.txt\n" +
				"rename to new name.txt\n",
			want: []diff.FileDiff{{
				OldPath: "old name.txt", NewPath: "new name.txt", Status: diff.StatusRenamed,
			}},
		},
		{
			name:       "rename with edits",
			nameStatus: "R90\x00old2.txt\x00new2.txt\x00",
			patch: "diff --git a/old2.txt b/new2.txt\n" +
				"similarity index 90%\n" +
				"rename from old2.txt\n" +
				"rename to new2.txt\n" +
				"index 1111111..2222222 100644\n" +
				"--- a/old2.txt\n" +
				"+++ b/new2.txt\n" +
				"@@ -1,2 +1,2 @@\n" +
				" line1\n" +
				"-line2\n" +
				"+line2 edited\n",
			want: []diff.FileDiff{{
				OldPath: "old2.txt", NewPath: "new2.txt", Status: diff.StatusRenamed,
				Added: 1, Deleted: 1,
				Hunks: []diff.Hunk{{
					OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2,
					Lines: []diff.Line{
						{Kind: diff.LineContext, OldNo: 1, NewNo: 1, Text: "line1"},
						{Kind: diff.LineDel, OldNo: 2, Text: "line2"},
						{Kind: diff.LineAdd, NewNo: 2, Text: "line2 edited"},
					},
				}},
			}},
		},
		{
			name:       "binary file",
			nameStatus: "A\x00image.png\x00",
			patch: "diff --git a/image.png b/image.png\n" +
				"new file mode 100644\n" +
				"index 0000000..3333333\n" +
				"Binary files /dev/null and b/image.png differ\n",
			want: []diff.FileDiff{{
				NewPath: "image.png", Status: diff.StatusAdded, Binary: true,
			}},
		},
		{
			name:       "no newline at end of file on the last added line",
			nameStatus: "M\x00noeol.txt\x00",
			patch: "diff --git a/noeol.txt b/noeol.txt\n" +
				"index 1111111..2222222 100644\n" +
				"--- a/noeol.txt\n" +
				"+++ b/noeol.txt\n" +
				"@@ -1 +1 @@\n" +
				"-old\n" +
				"+new\n" +
				"\\ No newline at end of file\n",
			want: []diff.FileDiff{{
				OldPath: "noeol.txt", NewPath: "noeol.txt", Status: diff.StatusModified,
				Added: 1, Deleted: 1,
				Hunks: []diff.Hunk{{
					OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1,
					Lines: []diff.Line{
						{Kind: diff.LineDel, OldNo: 1, Text: "old"},
						{Kind: diff.LineAdd, NewNo: 1, Text: "new", NoEOL: true},
					},
				}},
			}},
		},
		{
			name:       "two files, second has two hunks",
			nameStatus: "M\x00one.txt\x00M\x00two.txt\x00",
			patch: "diff --git a/one.txt b/one.txt\n" +
				"index 1111111..2222222 100644\n" +
				"--- a/one.txt\n" +
				"+++ b/one.txt\n" +
				"@@ -1 +1 @@\n" +
				"-a\n" +
				"+b\n" +
				"diff --git a/two.txt b/two.txt\n" +
				"index 3333333..4444444 100644\n" +
				"--- a/two.txt\n" +
				"+++ b/two.txt\n" +
				"@@ -1 +1 @@\n" +
				"-c\n" +
				"+d\n" +
				"@@ -10 +10 @@\n" +
				"-e\n" +
				"+f\n",
			want: []diff.FileDiff{
				{
					OldPath: "one.txt", NewPath: "one.txt", Status: diff.StatusModified,
					Added: 1, Deleted: 1,
					Hunks: []diff.Hunk{{
						OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1,
						Lines: []diff.Line{
							{Kind: diff.LineDel, OldNo: 1, Text: "a"},
							{Kind: diff.LineAdd, NewNo: 1, Text: "b"},
						},
					}},
				},
				{
					OldPath: "two.txt", NewPath: "two.txt", Status: diff.StatusModified,
					Added: 2, Deleted: 2,
					Hunks: []diff.Hunk{
						{
							OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1,
							Lines: []diff.Line{
								{Kind: diff.LineDel, OldNo: 1, Text: "c"},
								{Kind: diff.LineAdd, NewNo: 1, Text: "d"},
							},
						},
						{
							OldStart: 10, OldLines: 1, NewStart: 10, NewLines: 1,
							Lines: []diff.Line{
								{Kind: diff.LineDel, OldNo: 10, Text: "e"},
								{Kind: diff.LineAdd, NewNo: 10, Text: "f"},
							},
						},
					},
				},
			},
		},
		{
			name:       "count mismatch between the two inputs returns an error",
			nameStatus: "M\x00one.txt\x00M\x00two.txt\x00",
			patch: "diff --git a/one.txt b/one.txt\n" +
				"index 1111111..2222222 100644\n" +
				"--- a/one.txt\n" +
				"+++ b/one.txt\n" +
				"@@ -1 +1 @@\n" +
				"-a\n" +
				"+b\n",
			wantErr: "2 name-status entries but 1 patch sections",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := diff.Parse([]byte(tt.nameStatus), []byte(tt.patch))
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
