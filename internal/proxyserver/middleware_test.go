package proxyserver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

const testSecret = "test-secret"

func sign(key string) string {
	mac := hmac.New(sha256.New, []byte(testSecret))
	mac.Write([]byte(key))
	return hex.EncodeToString(mac.Sum(nil))
}

func newTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(NewAuthenticator(testSecret, AuthLocation.PathParams).Middleware())
	r.GET("/:key/:sign/websockify", func(c *gin.Context) { c.String(http.StatusOK, "ws") })
	r.GET("/:key/:sign/", func(c *gin.Context) { c.String(http.StatusOK, "index") })
	r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	return r
}

func TestMiddleware(t *testing.T) {
	r := newTestRouter()
	tests := []struct {
		name string
		path string
		want int
	}{
		{"ping without sign", "/ping", http.StatusOK},
		{"valid sign websockify", "/abc12345/" + sign("abc12345") + "/websockify", http.StatusOK},
		{"valid sign index", "/abc12345/" + sign("abc12345") + "/", http.StatusOK},
		{"wrong sign", "/abc12345/" + sign("other") + "/websockify", http.StatusForbidden},
		{"non-hex sign", "/abc12345/zz/websockify", http.StatusForbidden},
		{"empty sign", "/abc12345//websockify", http.StatusForbidden},
		{"ping key bypass websockify", "/ping/x/websockify", http.StatusForbidden},
		{"ping-prefixed key bypass websockify", "/pingx/x/websockify", http.StatusForbidden},
		{"ping key bypass index", "/ping/x/", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if w.Code != tt.want {
				t.Fatalf("GET %s: got %d, want %d", tt.path, w.Code, tt.want)
			}
		})
	}
}
