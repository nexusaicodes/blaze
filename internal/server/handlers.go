package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/sakshamsaxena/blaze/internal/plan"
	"github.com/sakshamsaxena/blaze/internal/state"
)

// sourceReader reads file content from git at a specific commit. *gitx.Runner
// satisfies this.
type sourceReader interface {
	Exists(ctx context.Context, sha, path string) bool
	Show(ctx context.Context, sha, path string) ([]byte, error)
}

// api holds what the HTTP handlers need: the plan, its pre-encoded JSON,
// the state store and a reader for source lines.
type api struct {
	plan     *plan.Plan
	planJSON []byte
	state    state.IStateStore
	git      sourceReader
}

// sourceResponse is the body of GET /api/source.
type sourceResponse struct {
	Side  string   `json:"side"`
	Path  string   `json:"path"`
	Start int      `json:"start"`
	Total int      `json:"total"`
	Lines []string `json:"lines"`
}

// handlePlan serves the plan, encoded once at startup.
func (a *api) handlePlan(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(a.planJSON)
}

// handleState serves the current review state.
func (a *api) handleState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.state.Snapshot())
}

// handleReview marks or unmarks an item as reviewed.
func (a *api) handleReview(w http.ResponseWriter, r *http.Request) {
	item, ok := a.plan.Items[r.PathValue("id")]
	if !ok {
		writeError(w, http.StatusNotFound, "unknown item")
		return
	}

	var body struct {
		Reviewed *bool `json:"reviewed"`
	}
	if !decodeJSON(r, &body) || body.Reviewed == nil {
		writeError(w, http.StatusBadRequest, `body must be {"reviewed": true} or {"reviewed": false}`)
		return
	}

	if err := a.state.SetReviewed(item.ContentHash, *body.Reviewed); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.state.Snapshot())
}

// handleNote sets or clears an item's note.
func (a *api) handleNote(w http.ResponseWriter, r *http.Request) {
	item, ok := a.plan.Items[r.PathValue("id")]
	if !ok {
		writeError(w, http.StatusNotFound, "unknown item")
		return
	}

	var body struct {
		Note *string `json:"note"`
	}
	if !decodeJSON(r, &body) || body.Note == nil {
		writeError(w, http.StatusBadRequest, `body must be {"note": "text"}`)
		return
	}
	if utf8.RuneCountInString(*body.Note) > 10000 {
		writeError(w, http.StatusBadRequest, "note too long")
		return
	}

	if err := a.state.SetNote(item.Key, *body.Note); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a.state.Snapshot())
}

// handleSource serves lines of one file at base or head.
func (a *api) handleSource(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	side := q.Get("side")
	if side != "head" && side != "base" {
		writeError(w, http.StatusBadRequest, "side must be head or base")
		return
	}

	p := q.Get("path")
	if !validSourcePath(p) {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}

	start, errStart := strconv.Atoi(q.Get("start"))
	end, errEnd := strconv.Atoi(q.Get("end"))
	if errStart != nil || errEnd != nil || start < 1 || end < start || end-start >= 400 {
		writeError(w, http.StatusBadRequest, "invalid start/end")
		return
	}

	sha := a.plan.Target.HeadSHA
	if side == "base" {
		sha = a.plan.Target.BaseSHA
	}

	ctx := r.Context()
	if !a.git.Exists(ctx, sha, p) {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}

	content, err := a.git.Show(ctx, sha, p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	probe := content
	if len(probe) > 8000 {
		probe = probe[:8000]
	}
	if bytes.IndexByte(probe, 0) >= 0 {
		writeError(w, http.StatusUnsupportedMediaType, "binary file")
		return
	}

	all := strings.Split(string(content), "\n")
	total := len(all)
	last := end
	if last > total {
		last = total
	}
	var lines []string
	if start <= total {
		lines = all[start-1 : last]
	}

	writeJSON(w, http.StatusOK, sourceResponse{
		Side:  side,
		Path:  p,
		Start: start,
		Total: total,
		Lines: nonNilStrings(lines),
	})
}

// validSourcePath reports whether p is a safe repo-relative path: non-empty,
// not absolute, equal to path.Clean(p), containing no ".." element and not
// starting with "-".
func validSourcePath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.HasPrefix(p, "-") {
		return false
	}
	if path.Clean(p) != p {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." {
			return false
		}
	}
	return true
}

// decodeJSON decodes r.Body into v, reporting whether it held exactly one
// valid JSON value.
func decodeJSON(r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		return false
	}
	return !dec.More()
}

// writeJSON encodes v as the JSON response body with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes the {"error": "<message>"} shape with the given status.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// nonNilStrings returns s, or an empty, non-nil slice when s is nil, so
// JSON encodes an empty list rather than null.
func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
