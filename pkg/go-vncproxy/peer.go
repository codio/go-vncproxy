package vncproxy

import (
	"io"
	"net"
	"time"

	"github.com/pkg/errors"

	"github.com/gorilla/websocket"
)

const (
	defaultDialTimeout = 5 * time.Second
	bufSize            = 32 * 1024
)

// Peer represents a vnc proxy peer
// with a websocket connection and a vnc backend connection
type Peer struct {
	source *websocket.Conn
	target net.Conn
	logger *logger
}

func NewPeer(ws *websocket.Conn, addr string, dialTimeout time.Duration, log *logger) (*Peer, error) {
	if ws == nil {
		return nil, errors.New("websocket connection is nil")
	}

	if len(addr) == 0 {
		return nil, errors.New("addr is empty")
	}

	if dialTimeout <= 0 {
		dialTimeout = defaultDialTimeout
	}
	c, err := net.DialTimeout("tcp", addr, dialTimeout)
	if err != nil {
		return nil, errors.Wrap(err, "cannot connect to vnc backend")
	}

	err = c.(*net.TCPConn).SetKeepAlive(true)
	if err != nil {
		return nil, errors.Wrap(err, "enable vnc backend connection keepalive failed")
	}

	err = c.(*net.TCPConn).SetKeepAlivePeriod(30 * time.Second)
	if err != nil {
		return nil, errors.Wrap(err, "set vnc backend connection keepalive period failed")
	}

	// 10 seconds timer to support connection open
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				pingErr := ws.WriteMessage(websocket.PingMessage, []byte{})
				if pingErr != nil {
					ws.Close()
				}
			}
		}
	}()

	return &Peer{
		source: ws,
		target: c,
		logger: log,
	}, nil
}

// ReadSource copy source stream to target connection
func (p *Peer) ReadSource() error {
	buf := make([]byte, bufSize)
	for {
		messageType, reader, err := p.source.NextReader()
		if err != nil {
			return errors.Wrapf(err, "read from source(%v) failed", p.source.RemoteAddr())
		}
		if messageType != websocket.BinaryMessage {
			continue
		}
		for {
			n, err2 := reader.Read(buf)
			if errors.Is(err2, io.EOF) {
				break
			}
			if err2 != nil {
				p.logger.Debugf("finished reading a websocket message, err: %v", err2)
				break
			}
			_, writeErr := p.target.Write(buf[:n])
			if writeErr != nil {
				return errors.Wrapf(writeErr, "write to target(%v) failed", p.target.RemoteAddr())
			}
		}
	}
}

// ReadTarget copy target stream to source connection
func (p *Peer) ReadTarget() error {
	buf := make([]byte, bufSize)
	for {
		n, err := p.target.Read(buf)
		if err != nil {
			return errors.Wrapf(err, "read from target(%v) failed", p.target.RemoteAddr())
		}
		writeErr := p.source.WriteMessage(websocket.BinaryMessage, buf[:n])
		if writeErr != nil {
			return errors.Wrapf(writeErr, "write to source(%v) failed", p.source.RemoteAddr())
		}
	}
}

// Close close the websocket connection and the vnc backend connection
func (p *Peer) Close() {
	p.source.Close()
	p.target.Close()
}
