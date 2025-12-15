package proxyserver

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"

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
}{
	listenPid:      "LISTEN_PID",
	authSignSecret: "GOVNC_SIGN_SECRET",
}

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
		LogLevel: vncproxy.DebugLevel,
		// DialTimeout: 10 * time.Second, // customer DialTimeout
		TokenHandler: func(r *http.Request) (addr string, err error) {
			return fmt.Sprintf(`:%d`, vncport), nil
		},
	})
}

func loadIndex(version string) (string, error) {
	resp, err := http.Get(fmt.Sprintf(`https://static-assets.codio.com/noVNC/%s/vnc.html`, version))
	if err != nil {
		return "", err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func runProxy(opts RunOpts) error {
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	pathPrefix := ""
	if opts.StrictAuth {
		secret := os.Getenv(envVariables.authSignSecret)
		if secret == "" {
			return fmt.Errorf("strict auth is enabled, but env variable %s is not set", envVariables.authSignSecret)
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

	if os.Getenv(envVariables.listenPid) == strconv.Itoa(os.Getpid()) {
		// systemd socket activation
		f := os.NewFile(3, "from systemd")
		listener, err := net.FileListener(f)
		if err != nil {
			return err
		}
		if err = router.RunListener(listener); err != nil {
			return err
		}
	} else {
		// cli activation
		if err := router.Run(fmt.Sprintf(":%d", opts.Port)); err != nil {
			return err
		}
	}
	return nil
}

func serveIndex(ctx *gin.Context) {
	ctx.Data(http.StatusOK, "text/html; charset=utf-8", []byte(indexHTML))
}
