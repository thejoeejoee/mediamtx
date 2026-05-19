// Package moq contains a Media over QUIC server.
package moq

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/google/uuid"
	"github.com/quic-go/webtransport-go"
)

// ErrSessionNotFound is returned when a session is not found.
var ErrSessionNotFound = errors.New("session not found")

type serverPathManager interface {
	Describe(req defs.PathDescribeReq) defs.PathDescribeRes
	AddReader(req defs.PathAddReaderReq) (*defs.PathAddReaderRes, error)
}

type newSessionRes struct {
	sx  *session
	err error
}

type newSessionReq struct {
	pathName string
	wt       *webtransport.Session
	res      chan newSessionRes
}

type serverAPISessionsListRes struct {
	data *defs.APIMOQSessionList
}

type serverAPISessionsListReq struct {
	res chan serverAPISessionsListRes
}

type serverAPISessionsGetRes struct {
	data *defs.APIMOQSession
	err  error
}

type serverAPISessionsGetReq struct {
	uuid uuid.UUID
	res  chan serverAPISessionsGetRes
}

type serverParent interface {
	logger.Writer
}

// Server is a Media over QUIC server.
type Server struct {
	HTTPS2Address  string
	HTTPS3Address  string
	ServerKey      string
	ServerCert     string
	AllowOrigins   []string
	TrustedProxies conf.IPNetworks
	ReadTimeout    conf.Duration
	WriteTimeout   conf.Duration
	PathManager    serverPathManager
	Parent         serverParent

	ctx        context.Context
	ctxCancel  context.CancelFunc
	httpServer *httpServer
	sessions   map[*session]struct{}

	chNewSession      chan newSessionReq
	chAPISessionsList chan serverAPISessionsListReq
	chAPISessionsGet  chan serverAPISessionsGetReq
	done              chan struct{}
}

// Initialize initializes the server.
func (s *Server) Initialize() error {
	s.httpServer = &httpServer{
		https2Address:  s.HTTPS2Address,
		https3Address:  s.HTTPS3Address,
		serverKey:      s.ServerKey,
		serverCert:     s.ServerCert,
		allowOrigins:   s.AllowOrigins,
		trustedProxies: s.TrustedProxies,
		readTimeout:    s.ReadTimeout,
		writeTimeout:   s.WriteTimeout,
		parent:         s,
	}
	err := s.httpServer.initialize()
	if err != nil {
		return err
	}

	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	s.sessions = make(map[*session]struct{})
	s.chNewSession = make(chan newSessionReq)
	s.chAPISessionsList = make(chan serverAPISessionsListReq)
	s.chAPISessionsGet = make(chan serverAPISessionsGetReq)
	s.done = make(chan struct{})

	s.Log(logger.Info, "started with listeners on %s (TCP/HTTPS2), %s (UDP/HTTP3)", s.HTTPS2Address, s.HTTPS3Address)

	go s.run()

	return nil
}

// Log implements logger.Writer.
func (s *Server) Log(level logger.Level, format string, args ...any) {
	s.Parent.Log(level, "[MoQ] "+format, args...)
}

// Close closes the server.
func (s *Server) Close() {
	s.Log(logger.Info, "closing")
	s.ctxCancel()
	<-s.done
}

func (s *Server) run() {
	defer close(s.done)

	var wg sync.WaitGroup

outer:
	for {
		select {
		case req := <-s.chNewSession:
			sx := &session{
				wt:          req.wt,
				wg:          &wg,
				pathName:    req.pathName,
				pathManager: s.PathManager,
				parent:      s,
			}
			sx.initialize()
			s.sessions[sx] = struct{}{}
			req.res <- newSessionRes{sx: sx}

		case req := <-s.chAPISessionsList:
			data := &defs.APIMOQSessionList{
				Items: []defs.APIMOQSession{},
			}
			for sx := range s.sessions {
				data.Items = append(data.Items, sx.apiItem())
			}
			req.res <- serverAPISessionsListRes{data: data}

		case req := <-s.chAPISessionsGet:
			var found *session
			for sx := range s.sessions {
				if sx.uuid == req.uuid {
					found = sx
					break
				}
			}
			if found == nil {
				req.res <- serverAPISessionsGetRes{err: ErrSessionNotFound}
			} else {
				item := found.apiItem()
				req.res <- serverAPISessionsGetRes{data: &item}
			}

		case <-s.ctx.Done():
			break outer
		}
	}

	// close sessions before closing UDP packet listener
	for sx := range s.sessions {
		sx.Close()
	}

	s.httpServer.close()

	wg.Wait()
}

func (s *Server) newSession(req newSessionReq) newSessionRes {
	req.res = make(chan newSessionRes)

	select {
	case s.chNewSession <- req:
		return <-req.res
	case <-s.ctx.Done():
		return newSessionRes{err: fmt.Errorf("terminated")}
	}
}

// APISessionsList implements defs.APIMOQServer.
func (s *Server) APISessionsList() (*defs.APIMOQSessionList, error) {
	req := serverAPISessionsListReq{
		res: make(chan serverAPISessionsListRes),
	}
	select {
	case s.chAPISessionsList <- req:
		res := <-req.res
		return res.data, nil
	case <-s.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}

// APISessionsGet implements defs.APIMOQServer.
func (s *Server) APISessionsGet(id uuid.UUID) (*defs.APIMOQSession, error) {
	req := serverAPISessionsGetReq{
		uuid: id,
		res:  make(chan serverAPISessionsGetRes),
	}
	select {
	case s.chAPISessionsGet <- req:
		res := <-req.res
		return res.data, res.err
	case <-s.ctx.Done():
		return nil, fmt.Errorf("terminated")
	}
}
