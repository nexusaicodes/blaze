package dodge

import (
	"context"
	"fmt"
)

// BuildInput is the input to Build.
type BuildInput struct {
	RepoRoot  string // absolute path of the head worktree
	ModuleDir string // go.mod directory relative to RepoRoot; "." for the root
}

// ParseInput is the input to ParseFile.
type ParseInput struct {
	Path       string // repo-relative path of the file
	Src        []byte
	ModulePath string
	ModuleDir  string
}

// IGraphProvider builds and extends the symbol graph for one module.
type IGraphProvider interface {
	// Build loads the module with full type information.
	Build(ctx context.Context, in BuildInput) (*Graph, error)
	// ParseFile extracts symbols from one file using syntax only.
	// The symbols have Resolved == false and no refs.
	ParseFile(in ParseInput) ([]*Symbol, error)
}

// New returns the IGraphProvider for backend. "go" returns the
// go/packages-backed provider; any other value is an error.
func New(backend string) (IGraphProvider, error) {
	switch backend {
	case "go":
		return &goProvider{}, nil
	default:
		return nil, fmt.Errorf("unknown dodge backend %q", backend)
	}
}

// goProvider implements IGraphProvider with go/ast and go/packages.
type goProvider struct{}
