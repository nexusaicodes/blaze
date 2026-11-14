package state

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sakshamsaxena/blaze/internal/logger"
)

func TestMain(m *testing.M) {
	logger.InitNop()
	os.Exit(m.Run())
}

// TestFileStoreRoundTrip sets marks and notes, constructs a new store on
// the same directory, opens it, and compares Snapshot.
func TestFileStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()

	s1 := &FileStore{baseDir: dir}
	require.NoError(t, s1.Open("repo1", "target1"))
	require.NoError(t, s1.SetReviewed("hash1", true))
	require.NoError(t, s1.SetReviewed("hash2", true))
	require.NoError(t, s1.SetNote("pkg.Foo", "check overflow"))

	s2 := &FileStore{baseDir: dir}
	require.NoError(t, s2.Open("repo1", "target1"))

	want := s1.Snapshot()
	got := s2.Snapshot()
	require.Equal(t, want, got)
	require.Len(t, got.Reviewed, 2)
	require.Equal(t, "check overflow", got.Notes["pkg.Foo"])
}

// TestFileStoreUnmarkAndEmptyNoteRemove checks that unmarking an item and
// setting an empty note both remove their entries.
func TestFileStoreUnmarkAndEmptyNoteRemove(t *testing.T) {
	dir := t.TempDir()
	s := &FileStore{baseDir: dir}
	require.NoError(t, s.Open("repo1", "target1"))

	require.NoError(t, s.SetReviewed("hash1", true))
	require.NoError(t, s.SetNote("pkg.Foo", "note"))
	require.Contains(t, s.Snapshot().Reviewed, "hash1")
	require.Contains(t, s.Snapshot().Notes, "pkg.Foo")

	require.NoError(t, s.SetReviewed("hash1", false))
	require.NoError(t, s.SetNote("pkg.Foo", ""))

	snap := s.Snapshot()
	require.NotContains(t, snap.Reviewed, "hash1")
	require.NotContains(t, snap.Notes, "pkg.Foo")
}

// TestFileStoreCorruptFile checks that a file that fails to decode is
// renamed to <name>.corrupt and replaced by an empty state.
func TestFileStoreCorruptFile(t *testing.T) {
	dir := t.TempDir()
	targetDir := filepath.Join(dir, "blaze", "state", "repo1")
	require.NoError(t, os.MkdirAll(targetDir, 0o700))
	path := filepath.Join(targetDir, "target1.json")
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0o600))

	s := &FileStore{baseDir: dir}
	require.NoError(t, s.Open("repo1", "target1"))

	require.Equal(t, emptyState(), s.Snapshot())
	require.FileExists(t, path+".corrupt")
	require.NoFileExists(t, path)
}

// TestFileStoreSnapshotIsDeepCopy checks that mutating a Snapshot does not
// affect the store.
func TestFileStoreSnapshotIsDeepCopy(t *testing.T) {
	dir := t.TempDir()
	s := &FileStore{baseDir: dir}
	require.NoError(t, s.Open("repo1", "target1"))
	require.NoError(t, s.SetReviewed("hash1", true))

	snap := s.Snapshot()
	snap.Reviewed["hash2"] = Mark{}
	snap.Notes["x"] = "y"
	delete(snap.Reviewed, "hash1")

	after := s.Snapshot()
	require.Contains(t, after.Reviewed, "hash1")
	require.NotContains(t, after.Reviewed, "hash2")
	require.NotContains(t, after.Notes, "x")
}

// TestFileStoreConcurrentSetReviewed exercises concurrent SetReviewed calls
// under -race.
func TestFileStoreConcurrentSetReviewed(t *testing.T) {
	dir := t.TempDir()
	s := &FileStore{baseDir: dir}
	require.NoError(t, s.Open("repo1", "target1"))

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			hash := "hash" + string(rune('a'+i%26))
			require.NoError(t, s.SetReviewed(hash, true))
		}(i)
	}
	wg.Wait()
}
