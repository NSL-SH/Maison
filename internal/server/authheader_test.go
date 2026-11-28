package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/yundera/maison/internal/brand"
	"github.com/yundera/maison/internal/config"
	"github.com/yundera/maison/internal/routing"
	"github.com/yundera/maison/internal/usersettings"
)

// A dashboard response carries X-Maison-Auth exactly when the proxy in front
// vouched for the user (a forward-auth identity header), and never otherwise.
func TestAuthHeader(t *testing.T) {
	cfg := config.Config{DataRoot: t.TempDir(), Routing: func() routing.Setting { return routing.Setting{} }}
	s := &Server{cfg: cfg, settings: usersettings.New(filepath.Join(cfg.StateDir(), "settings.json"))}
	h := s.rootHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, c := range []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{"no identity", nil, ""},
		{"Remote-User", map[string]string{"Remote-User": "alice"}, "1"},
		{"X-Forwarded-User", map[string]string{"X-Forwarded-User": "alice"}, "1"},
		{"blank identity", map[string]string{"Remote-User": "  "}, ""},
		{"other headers only", map[string]string{"X-Forwarded-Email": "a@b.c"}, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			for k, v := range c.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if got := rec.Header().Get(brand.AuthHeader); got != c.want {
				t.Errorf("%s = %q, want %q", brand.AuthHeader, got, c.want)
			}
			if rec.Header().Get(brand.Header) != "1" {
				t.Errorf("%s missing", brand.Header)
			}
		})
	}
}

func TestAuthHeaderName(t *testing.T) {
	if got := http.CanonicalHeaderKey(brand.AuthHeader); got != "X-Maison-Auth" {
		t.Errorf("AuthHeader on the wire = %q, want X-Maison-Auth", got)
	}
}
