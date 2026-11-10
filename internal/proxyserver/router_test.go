package proxyserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

const testAncestors = "https://codio.com https://*.codio.com"

func newTestServerRouter(t *testing.T, strict bool, frameAncestors string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	t.Setenv(envVariables.authSignSecret, testSecret)
	t.Setenv(envVariables.frameAncestors, frameAncestors)
	r, err := newRouter(RunOpts{VncPort: 5900, StrictAuth: strict})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func get(r *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestFrameAncestorsHeader(t *testing.T) {
	r := newTestServerRouter(t, true, " "+testAncestors+" ")
	want := "frame-ancestors " + testAncestors
	paths := map[string]int{
		"/abc12345/" + sign("abc12345") + "/":           http.StatusOK,
		"/abc12345/" + sign("abc12345") + "/index.html": http.StatusOK,
		"/abc12345/bad/":     http.StatusForbidden,
		"/ping":              http.StatusOK,
		"/abc12345/x/y/z/w/": http.StatusForbidden,
	}
	for path, status := range paths {
		w := get(r, path)
		if w.Code != status {
			t.Errorf("GET %s: got %d, want %d", path, w.Code, status)
		}
		if got := w.Header().Get("Content-Security-Policy"); got != want {
			t.Errorf("GET %s: CSP %q, want %q", path, got, want)
		}
	}
}

func TestNoFrameAncestorsHeaderWhenUnset(t *testing.T) {
	r := newTestServerRouter(t, false, "")
	for _, path := range []string{"/", "/index.html", "/ping", "/missing"} {
		if got := get(r, path).Header().Get("Content-Security-Policy"); got != "" {
			t.Errorf("GET %s: unexpected CSP %q", path, got)
		}
	}
}

func TestErrorPages(t *testing.T) {
	tests := []struct {
		name   string
		strict bool
		path   string
		status int
		text   string
	}{
		{"forbidden", true, "/abc12345/bad/", http.StatusForbidden, "Session link is invalid or expired"},
		{"forbidden websockify", true, "/abc12345/bad/websockify", http.StatusForbidden, "Session link is invalid or expired"},
		{"not found", false, "/missing", http.StatusNotFound, "Page not found"},
		{"not found behind valid sign", true, "/abc12345/" + sign("abc12345") + "/missing", http.StatusNotFound, "Page not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := get(newTestServerRouter(t, tt.strict, ""), tt.path)
			if w.Code != tt.status {
				t.Fatalf("got %d, want %d", w.Code, tt.status)
			}
			if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
				t.Fatalf("content type %q, want text/html", ct)
			}
			if !strings.Contains(w.Body.String(), tt.text) {
				t.Fatalf("body does not contain %q", tt.text)
			}
		})
	}
}

func TestErrorPageDoesNotReflectPath(t *testing.T) {
	w := get(newTestServerRouter(t, false, ""), "/%3Cscript%3Ealert(1)%3C%2Fscript%3E")
	if strings.Contains(w.Body.String(), "<script") {
		t.Fatal("error page reflects request path")
	}
}

func TestInvalidFrameAncestorsRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, v := range []string{
		"https://codio.com; script-src *",
		"https://codio.com, https://evil.com",
		"https://codio.com\r\nX-Injected: 1",
	} {
		t.Setenv(envVariables.frameAncestors, v)
		if _, err := newRouter(RunOpts{}); err == nil {
			t.Errorf("value %q accepted, want error", v)
		}
	}
}

func TestStrictAuthRequiresSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(envVariables.authSignSecret, "")
	t.Setenv(envVariables.frameAncestors, "")
	if _, err := newRouter(RunOpts{StrictAuth: true}); err == nil {
		t.Fatal("strict auth without secret accepted")
	}
}
