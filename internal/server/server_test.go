package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sakshamsaxena/blaze/internal/logger"
	"github.com/sakshamsaxena/blaze/internal/plan"
	"github.com/sakshamsaxena/blaze/internal/state"
	"github.com/sakshamsaxena/blaze/internal/target"
)

func TestMain(m *testing.M) {
	logger.InitNop()
	os.Exit(m.Run())
}

// stubGit is a hand-built sourceReader for tests.
type stubGit struct {
	content map[string][]byte // key: sha+":"+path
	showErr error
}

func (g *stubGit) key(sha, path string) string { return sha + ":" + path }

func (g *stubGit) Exists(ctx context.Context, sha, path string) bool {
	_, ok := g.content[g.key(sha, path)]
	return ok
}

func (g *stubGit) Show(ctx context.Context, sha, path string) ([]byte, error) {
	if g.showErr != nil {
		return nil, g.showErr
	}
	return g.content[g.key(sha, path)], nil
}

// testPlan returns a hand-built two-item plan for the server tests.
func testPlan() *plan.Plan {
	return &plan.Plan{
		Version: 1,
		Target:  target.Target{BaseSHA: "base123", HeadSHA: "head456"},
		Items: map[string]*plan.Item{
			"item1": {ID: "item1", Key: "pkg.Foo", ContentHash: "hash1"},
			"item2": {ID: "item2", Key: "pkg.Bar", ContentHash: "hash2"},
		},
	}
}

// testServer builds a handler and wraps it in an httptest.Server bound to
// the port the handler's host check expects.
func testServer(t *testing.T, p *plan.Plan, st state.IStateStore, git sourceReader, token string) *httptest.Server {
	t.Helper()
	ts := httptest.NewUnstartedServer(http.NotFoundHandler())
	port := ts.Listener.Addr().(*net.TCPAddr).Port

	handler, err := buildHandler(p, st, git, port, token)
	require.NoError(t, err)
	ts.Config.Handler = handler
	ts.Start()
	t.Cleanup(ts.Close)
	return ts
}

func newMemoryState(t *testing.T) state.IStateStore {
	t.Helper()
	st, err := state.New("memory")
	require.NoError(t, err)
	require.NoError(t, st.Open("repo", "target"))
	return st
}

func doRequest(t *testing.T, ts *httptest.Server, method, path, token string, body []byte) *http.Response {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, ts.URL+path, reader)
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("X-Blaze-Token", token)
	}
	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	return resp
}

// TestWrongHostForbidden checks that a request with the wrong Host header
// is rejected with 403, without reaching the application.
func TestWrongHostForbidden(t *testing.T) {
	ts := testServer(t, testPlan(), newMemoryState(t), &stubGit{}, "tok")

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/plan", nil)
	require.NoError(t, err)
	req.Host = "evil.example.com"
	req.Header.Set("X-Blaze-Token", "tok")

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

// TestPlanRequiresToken checks GET /api/plan: 403 without the token, 200
// with valid JSON with it.
func TestPlanRequiresToken(t *testing.T) {
	p := testPlan()
	ts := testServer(t, p, newMemoryState(t), &stubGit{}, "tok")

	resp := doRequest(t, ts, http.MethodGet, "/api/plan", "", nil)
	defer resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	resp2 := doRequest(t, ts, http.MethodGet, "/api/plan", "tok", nil)
	defer resp2.Body.Close()
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	var got plan.Plan
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&got))
	require.Equal(t, p.Target.BaseSHA, got.Target.BaseSHA)
}

// TestReviewMutatesState checks PUT review: state changes; unknown id is
// 404; a bad body is 400.
func TestReviewMutatesState(t *testing.T) {
	ts := testServer(t, testPlan(), newMemoryState(t), &stubGit{}, "tok")

	resp := doRequest(t, ts, http.MethodPut, "/api/items/item1/review", "tok", []byte(`{"reviewed": true}`))
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got state.State
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Contains(t, got.Reviewed, "hash1")

	respUnknown := doRequest(t, ts, http.MethodPut, "/api/items/nope/review", "tok", []byte(`{"reviewed": true}`))
	defer respUnknown.Body.Close()
	require.Equal(t, http.StatusNotFound, respUnknown.StatusCode)

	respBad := doRequest(t, ts, http.MethodPut, "/api/items/item1/review", "tok", []byte(`not json`))
	defer respBad.Body.Close()
	require.Equal(t, http.StatusBadRequest, respBad.StatusCode)
}

// TestNoteOverLimitRejected checks that a note over 10 000 runes is
// rejected with 400.
func TestNoteOverLimitRejected(t *testing.T) {
	ts := testServer(t, testPlan(), newMemoryState(t), &stubGit{}, "tok")

	ok := doRequest(t, ts, http.MethodPut, "/api/items/item1/note", "tok", []byte(`{"note": "fine"}`))
	defer ok.Body.Close()
	require.Equal(t, http.StatusOK, ok.StatusCode)

	long := `{"note": "` + strings.Repeat("a", 10001) + `"}`
	resp := doRequest(t, ts, http.MethodPut, "/api/items/item1/note", "tok", []byte(long))
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestSourceValidation checks that /api/source rejects a path with "..", an
// absolute path, end < start and a range of 400 lines or more.
func TestSourceValidation(t *testing.T) {
	ts := testServer(t, testPlan(), newMemoryState(t), &stubGit{}, "tok")

	cases := []struct {
		name  string
		query string
	}{
		{"dotdot", "side=head&path=../x&start=1&end=2"},
		{"absolute", "side=head&path=/etc/passwd&start=1&end=2"},
		{"end-before-start", "side=head&path=f.go&start=5&end=2"},
		{"range-too-large", "side=head&path=f.go&start=1&end=401"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := doRequest(t, ts, http.MethodGet, "/api/source?"+c.query, "tok", nil)
			defer resp.Body.Close()
			require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	}
}

// TestSourceServesLines checks the success path of /api/source.
func TestSourceServesLines(t *testing.T) {
	git := &stubGit{content: map[string][]byte{
		"head456:f.go": []byte("one\ntwo\nthree\n"),
	}}
	ts := testServer(t, testPlan(), newMemoryState(t), git, "tok")

	resp := doRequest(t, ts, http.MethodGet, "/api/source?side=head&path=f.go&start=1&end=2", "tok", nil)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got sourceResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, []string{"one", "two"}, got.Lines)
	require.Equal(t, 4, got.Total)
}

// TestIndexHasToken checks that the index response contains the token and
// no literal placeholder text.
func TestIndexHasToken(t *testing.T) {
	ts := testServer(t, testPlan(), newMemoryState(t), &stubGit{}, "sekret")

	resp := doRequest(t, ts, http.MethodGet, "/", "", nil)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(resp.Body)
	require.NoError(t, err)
	body := buf.String()

	require.Contains(t, body, "sekret")
	require.NotContains(t, body, "__BLAZE_TOKEN__")
}

// TestSecurityHeadersAlwaysPresent checks that every response, including a
// 403, carries the four security headers.
func TestSecurityHeadersAlwaysPresent(t *testing.T) {
	ts := testServer(t, testPlan(), newMemoryState(t), &stubGit{}, "tok")

	check := func(resp *http.Response) {
		require.NotEmpty(t, resp.Header.Get("Content-Security-Policy"))
		require.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
		require.Equal(t, "no-referrer", resp.Header.Get("Referrer-Policy"))
		require.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
	}

	resp1 := doRequest(t, ts, http.MethodGet, "/api/plan", "tok", nil)
	defer resp1.Body.Close()
	check(resp1)

	resp2 := doRequest(t, ts, http.MethodGet, "/api/plan", "", nil)
	defer resp2.Body.Close()
	check(resp2)
}
