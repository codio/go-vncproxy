package vncproxy

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func startBackend(t *testing.T, handle func(net.Conn)) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go handle(c)
		}
	}()
	return l.Addr().String()
}

func startProxy(t *testing.T, backendAddr string) string {
	t.Helper()
	p := New(&Config{
		TokenHandler: func(*http.Request) (string, error) { return backendAddr, nil },
	})
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = ws.Close() }()
		p.ServeWS(ws, r)
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func dial(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	return ws
}

func TestProxyRelaysBothWays(t *testing.T) {
	backend := startBackend(t, func(c net.Conn) {
		defer func() { _ = c.Close() }()
		_, _ = io.Copy(c, c)
	})
	ws := dial(t, startProxy(t, backend))

	payload := bytes.Repeat([]byte("vnc"), 50_000)
	if err := ws.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		t.Fatal(err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	var got []byte
	for len(got) < len(payload) {
		mt, data, err := ws.ReadMessage()
		if err != nil {
			t.Fatalf("read after %d bytes: %v", len(got), err)
		}
		if mt != websocket.BinaryMessage {
			t.Fatalf("message type %d, want binary", mt)
		}
		got = append(got, data...)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("echoed payload differs")
	}
}

func TestProxyClosesWebsocketWhenBackendCloses(t *testing.T) {
	backend := startBackend(t, func(c net.Conn) {
		_, _ = c.Write([]byte("RFB 003.008\n"))
		_ = c.Close()
	})
	ws := dial(t, startProxy(t, backend))

	_ = ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, data, err := ws.ReadMessage(); err != nil || string(data) != "RFB 003.008\n" {
		t.Fatalf("first message: %q, %v", data, err)
	}
	_, _, err := ws.ReadMessage()
	if err == nil {
		t.Fatal("expected websocket to be closed after backend closed")
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		t.Fatal("websocket stayed open after backend closed")
	}
}
