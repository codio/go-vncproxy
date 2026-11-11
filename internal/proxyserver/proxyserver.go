package proxyserver

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/codio/go-vncproxy/pkg/go-vncproxy"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var envVariables = struct {
	listenPid      string
	authSignSecret string
	frameAncestors string
}{
	listenPid:      "LISTEN_PID",
	authSignSecret: "GOVNC_SIGN_SECRET",
	frameAncestors: "GOVNC_FRAME_ANCESTORS",
}

const (
	indexLoadTimeout  = 30 * time.Second
	readHeaderTimeout = 10 * time.Second
	idleTimeout       = 120 * time.Second
)

var indexHTML string

func Run(opts RunOpts) error {
	var err error
	indexHTML, err = loadIndex(opts.NoVncVersion)
	if err != nil {
		return fmt.Errorf("failed to load index.html: %w", err)
	}
	return runProxy(opts)
}

func NewVNCProxy(vncport int) *vncproxy.Proxy {
	return vncproxy.New(&vncproxy.Config{
		LogLevel: vncproxy.InfoLevel,
		// DialTimeout: 10 * time.Second, // customer DialTimeout
		TokenHandler: func(r *http.Request) (addr string, err error) {
			return fmt.Sprintf(`:%d`, vncport), nil
		},
	})
}

func loadIndex(version string) (string, error) {
	client := &http.Client{Timeout: indexLoadTimeout}
	resp, err := client.Get(fmt.Sprintf(`https://static-assets.codio.com/noVNC/%s/vnc.html`, version))
	if err != nil {
		return "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func runProxy(opts RunOpts) error {
	gin.SetMode(gin.ReleaseMode)
	router, err := newRouter(opts)
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", opts.Port),
		Handler:           router.Handler(),
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
	}

	if os.Getenv(envVariables.listenPid) == strconv.Itoa(os.Getpid()) {
		// systemd socket activation
		f := os.NewFile(3, "from systemd")
		listener, err := net.FileListener(f)
		if err != nil {
			return err
		}
		return server.Serve(listener)
	}
	// cli activation
	return server.ListenAndServe()
}

func newRouter(opts RunOpts) (*gin.Engine, error) {
	router := gin.New()
	router.Use(gin.Recovery())

	if frameAncestors := os.Getenv(envVariables.frameAncestors); frameAncestors != "" {
		cspMiddleware, err := frameAncestorsMiddleware(frameAncestors)
		if err != nil {
			return nil, fmt.Errorf("invalid %s: %w", envVariables.frameAncestors, err)
		}
		router.Use(cspMiddleware)
	}
	router.NoRoute(func(c *gin.Context) {
		abortWithErrorPage(c, http.StatusNotFound)
	})

	pathPrefix := ""
	if opts.StrictAuth {
		secret := os.Getenv(envVariables.authSignSecret)
		if secret == "" {
			return nil, fmt.Errorf("strict auth is enabled, but env variable %s is not set", envVariables.authSignSecret)
		}
		authLocation := AuthLocation.PathParams
		if authLocation == AuthLocation.PathParams {
			pathPrefix = "/:key/:sign"
		}
		authenticator := NewAuthenticator(secret, authLocation)
		router.Use(authenticator.Middleware())
	}

	vncProxy := NewVNCProxy(opts.VncPort)
	router.GET(fmt.Sprintf("%s/websockify", pathPrefix), func(ctx *gin.Context) {
		conn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
		if err != nil {
			ctx.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		defer func(conn *websocket.Conn) {
			_ = conn.Close()
		}(conn)
		vncProxy.ServeWS(conn, ctx.Request)
	})

	router.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	router.GET(fmt.Sprintf("%s/index.html", pathPrefix), serveIndex)
	router.GET(fmt.Sprintf("%s/", pathPrefix), serveIndex)

	return router, nil
}

func serveIndex(ctx *gin.Context) {
	ctx.Data(http.StatusOK, "text/html; charset=utf-8", []byte(indexHTML))
}
