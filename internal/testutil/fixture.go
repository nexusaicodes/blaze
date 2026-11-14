// Package testutil builds the small fixture git repository shared by the
// gitx, target, diff and plan tests.
package testutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Fixture is a git repository with a base commit on main and a head commit
// on feature.
type Fixture struct {
	Dir     string // repository root (symlinks resolved)
	BaseSHA string // commit on main
	HeadSHA string // commit on feature
}

// gitConfigArgs are prepended to every git command the builder runs so the
// developer's own configuration cannot interfere.
var gitConfigArgs = []string{
	"-c", "user.name=blaze",
	"-c", "user.email=blaze@example.invalid",
	"-c", "commit.gpgsign=false",
	"-c", "core.hooksPath=/dev/null",
}

// NewFixture creates the fixture repository in t.TempDir() and returns it.
// It skips the test when git or go is not on PATH.
func NewFixture(t *testing.T) Fixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}

	dir := t.TempDir()
	resolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	writeBaseFiles(t, resolved)
	runGit(t, resolved, "init", "--quiet", "--initial-branch=main")
	runGit(t, resolved, "add", "-A")
	runGit(t, resolved, "commit", "-m", "base")
	runGit(t, resolved, "checkout", "--quiet", "-b", "feature")

	applyHeadChanges(t, resolved)
	runGit(t, resolved, "add", "-A")
	runGit(t, resolved, "commit", "-m", "head")

	baseSHA := strings.TrimSpace(string(runGitOutput(t, resolved, "rev-parse", "main")))
	headSHA := strings.TrimSpace(string(runGitOutput(t, resolved, "rev-parse", "feature")))

	return Fixture{Dir: resolved, BaseSHA: baseSHA, HeadSHA: headSHA}
}

// runGit runs a git command in dir with the fixture's fixed identity
// config, failing the test on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	runGitOutput(t, dir, args...)
}

// runGitOutput is runGit but returns combined output.
func runGitOutput(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	full := append(append([]string{}, gitConfigArgs...), args...)
	cmd := exec.CommandContext(context.Background(), "git", full...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v: %s", args, out)
	return out
}

// writeFile writes content to rel under dir, creating parent directories.
func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func writeBaseFiles(t *testing.T, dir string) {
	writeFile(t, dir, "go.mod", baseGoMod)
	writeFile(t, dir, "README.md", baseReadme)
	writeFile(t, dir, "old name.txt", oldNameTxt)
	writeFile(t, dir, "store/store.go", baseStoreGo)
	writeFile(t, dir, "store/store_test.go", baseStoreTestGo)
	writeFile(t, dir, "store/ext_test.go", extTestGo)
	writeFile(t, dir, "service/service.go", baseServiceGo)
	writeFile(t, dir, "cmd/app/main.go", mainGo)
}

func applyHeadChanges(t *testing.T, dir string) {
	writeFile(t, dir, "store/store.go", headStoreGo)
	writeFile(t, dir, "service/service.go", headServiceGo)
	writeFile(t, dir, "store/store_test.go", headStoreTestGo)
	writeFile(t, dir, "README.md", headReadme)
	runGit(t, dir, "mv", "old name.txt", "new name.txt")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bin.dat"), []byte{0x00, 0x01}, 0o644))
}

// Base file contents. Tabs matter; copied exactly from 08-fixture.md.
const (
	baseGoMod = `module example.com/fixture

go 1.25
`

	baseReadme = `# fixture
`

	headReadme = `# fixture

Now with delete.
`

	oldNameTxt = `x
`

	baseStoreGo = `package store

import "errors"

// ErrNotFound is returned when a record is missing.
var ErrNotFound = errors.New("not found")

// Record is one stored item.
type Record struct {
	ID   string
	Body string
}

// Store persists records.
type Store interface {
	Save(r Record) error
	Load(id string) (Record, error)
}

// MemStore is an in-memory Store.
type MemStore struct {
	data map[string]Record
}

// NewMemStore builds an empty MemStore.
func NewMemStore() *MemStore {
	return &MemStore{data: map[string]Record{}}
}

// Save stores r.
func (m *MemStore) Save(r Record) error {
	m.data[r.ID] = r
	return nil
}

// Load fetches a record.
func (m *MemStore) Load(id string) (Record, error) {
	r, ok := m.data[id]
	if !ok {
		return Record{}, ErrNotFound
	}
	return r, nil
}
`

	headStoreGo = `package store

import "errors"

// ErrNotFound is returned when a record is missing.
var ErrNotFound = errors.New("not found")

// Record is one stored item.
type Record struct {
	ID   string
	Body string
	Version int
}

// Store persists records.
type Store interface {
	Save(r Record) error
	Load(id string) (Record, error)
	Delete(id string) error
}

// MemStore is an in-memory Store.
type MemStore struct {
	data map[string]Record
}

// NewMemStore builds an empty MemStore.
func NewMemStore() *MemStore {
	return &MemStore{data: map[string]Record{}}
}

// Save stores r.
func (m *MemStore) Save(r Record) error {
	r.Version = m.data[r.ID].Version + 1
	m.data[r.ID] = r
	return nil
}

// Load fetches a record.
func (m *MemStore) Load(id string) (Record, error) {
	r, ok := m.data[id]
	if !ok {
		return Record{}, ErrNotFound
	}
	return r, nil
}

// Delete removes a record.
func (m *MemStore) Delete(id string) error {
	delete(m.data, id)
	return nil
}
`

	baseStoreTestGo = `package store

import "testing"

func TestSave(t *testing.T) {
	m := NewMemStore()
	if err := m.Save(Record{ID: "a"}); err != nil {
		t.Fatal(err)
	}
}
`

	headStoreTestGo = `package store

import "testing"

func TestSave(t *testing.T) {
	m := NewMemStore()
	if err := m.Save(Record{ID: "a"}); err != nil {
		t.Fatal(err)
	}
}

func TestDelete(t *testing.T) {
	m := NewMemStore()
	if err := m.Delete("a"); err != nil {
		t.Fatal(err)
	}
}
`

	extTestGo = `package store_test

import (
	"testing"

	"example.com/fixture/store"
)

func TestLoadMissing(t *testing.T) {
	m := store.NewMemStore()
	if _, err := m.Load("x"); err != store.ErrNotFound {
		t.Fatal(err)
	}
}
`

	baseServiceGo = `package service

import "example.com/fixture/store"

// Service is the business layer.
type Service struct {
	st store.Store
}

// New wires a Service.
func New(st store.Store) *Service {
	return &Service{st: st}
}

// Put writes a record.
func (s *Service) Put(id, body string) error {
	return s.st.Save(store.Record{ID: id, Body: body})
}

// Get reads a record body.
func (s *Service) Get(id string) (string, error) {
	r, err := s.st.Load(id)
	if err != nil {
		return "", err
	}
	return r.Body, nil
}
`

	headServiceGo = `package service

import "example.com/fixture/store"

// Service is the business layer.
type Service struct {
	st store.Store
}

// New wires a Service.
func New(st store.Store) *Service {
	return &Service{st: st}
}

// Put writes a record.
func (s *Service) Put(id, body string) error {
	return s.st.Save(store.Record{ID: id, Body: body})
}

// Get reads a record body.
func (s *Service) Get(id string) (string, error) {
	r, err := s.st.Load(id)
	if err != nil {
		return "", err
	}
	return r.Body, nil
}

// Remove deletes a record.
func (s *Service) Remove(id string) error {
	return s.st.Delete(id)
}
`

	mainGo = `package main

import (
	"fmt"

	"example.com/fixture/service"
	"example.com/fixture/store"
)

func main() {
	svc := service.New(store.NewMemStore())
	if err := svc.Put("a", "hello"); err != nil {
		panic(err)
	}
	fmt.Println(svc.Get("a"))
}
`
)
