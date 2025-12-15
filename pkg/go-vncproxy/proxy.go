package vncproxy

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type TokenHandler func(r *http.Request) (addr string, err error)

// Config represents vnc proxy config
type Config struct {
	LogLevel    uint32
	Logger      Logger
	DialTimeout time.Duration
	TokenHandler
	ErrorCh chan error
}

// Proxy represents vnc proxy
type Proxy struct {
	logLevel     uint32
	logger       *logger
	dialTimeout  time.Duration // Timeout for connecting to each target vnc server
	peers        map[*Peer]struct{}
	l            sync.RWMutex
	tokenHandler TokenHandler
	errorCh      chan error
}

var (
	ErrGetVNCBackend = errors.New("failed to get VNC backend")
	ErrNewVNCPeer    = errors.New("failed to create VNC peer")
	ErrReadSource    = errors.New("failed to read source")
	ErrReadTarget    = errors.New("failed to read target")
)

// New returns a vnc proxy
// If token handler is nil, vnc backend address will always be :5901
func New(conf *Config) *Proxy {
	if conf.TokenHandler == nil {
		conf.TokenHandler = func(r *http.Request) (addr string, err error) {
			return ":5901", nil
		}
	}

	return &Proxy{
		logLevel:     conf.LogLevel,
		logger:       NewLogger(conf.LogLevel, conf.Logger),
		dialTimeout:  conf.DialTimeout,
		peers:        make(map[*Peer]struct{}),
		l:            sync.RWMutex{},
		tokenHandler: conf.TokenHandler,
		errorCh:      conf.ErrorCh,
	}
}

// ServeWS provides websocket handler
func (p *Proxy) ServeWS(ws *websocket.Conn, request *http.Request) {
	p.logger.Debugf("ServeWS")
	p.logger.Debugf("request url: %v", request.URL)
	// get vnc backend server addr
	addr, err := p.tokenHandler(request)
	if err != nil {
		p.pushErrorTop(ErrGetVNCBackend)
		p.logger.Infof("get vnc backend failed: %v", err)
		return
	}

	peer, err := NewPeer(ws, addr, p.dialTimeout, p.logger)
	if err != nil {
		p.pushErrorTop(ErrNewVNCPeer)
		p.logger.Infof("new vnc peer failed: %v", err)
		return
	}

	p.addPeer(peer)
	defer func() {
		p.logger.Info("close peer")
		p.deletePeer(peer)
	}()

	go func() {
		if err2 := peer.ReadTarget(); err2 != nil {
			if strings.Contains(err2.Error(), "use of closed network connection") {
				return
			}
			p.pushErrorTop(ErrReadTarget)
			p.logger.Info(err2)
			return
		}
	}()

	if err = peer.ReadSource(); err != nil {
		if strings.Contains(err.Error(), "use of closed network connection") {
			return
		}
		p.pushErrorTop(ErrReadSource)
		p.logger.Info(err)
		return
	}
}

func (p *Proxy) addPeer(peer *Peer) {
	p.l.Lock()
	p.peers[peer] = struct{}{}
	p.l.Unlock()
}

func (p *Proxy) deletePeer(peer *Peer) {
	p.l.Lock()
	delete(p.peers, peer)
	peer.Close()
	p.l.Unlock()
}

func (p *Proxy) Peers() map[*Peer]struct{} {
	p.l.RLock()
	defer p.l.RUnlock()
	return p.peers
}

func (p *Proxy) pushErrorTop(err error) {
	if p.errorCh != nil {
		p.errorCh <- err
	}
}
