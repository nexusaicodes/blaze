package server

import (
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"
)

// csp is the Content-Security-Policy applied to every response.
const csp = "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; base-uri 'none'; form-action 'none'"

// withSecurity wraps h with the host check, the security response headers
// and the API token check, in that order. port is the listener's bound
// port; token is the per-run API token.
func withSecurity(h http.Handler, port int, token string) http.Handler {
	allowedHosts := map[string]bool{
		"127.0.0.1:" + strconv.Itoa(port): true,
		"localhost:" + strconv.Itoa(port): true,
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")

		if !allowedHosts[r.Host] {
			writeError(w, http.StatusForbidden, "forbidden host")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)

		if strings.HasPrefix(r.URL.Path, "/api/") {
			got := r.Header.Get("X-Blaze-Token")
			if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				writeError(w, http.StatusForbidden, "forbidden")
				return
			}
		}

		h.ServeHTTP(w, r)
	})
}
