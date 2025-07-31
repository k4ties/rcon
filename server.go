package rcon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/df-mc/dragonfly/server/event"
	"github.com/gorcon/rcon"
	"github.com/k4ties/gq"
	"github.com/sasha-s/go-deadlock"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// Server is an RCON server listening on a system-chosen port on the local
// loop-back interface.
type Server struct {
	password []byte

	listener net.Listener
	logger   *slog.Logger

	conn   gq.Map[net.Addr, *Conn]
	connMu deadlock.RWMutex

	close    chan struct{}
	incoming chan net.Conn

	wg   sync.WaitGroup
	once sync.Once

	handler atomic.Pointer[Handler]
}

// NewServer creates new Server instance.
func NewServer(password []byte, opts ...ServerOption) *Server {
	srv := &Server{
		password: password,
		conn:     make(gq.Map[net.Addr, *Conn]),
		close:    make(chan struct{}, 1),
		incoming: make(chan net.Conn),
	}
	// Applying options
	for _, opt := range opts {
		opt(srv)
	}
	if srv.logger == nil {
		srv.logger = slog.Default()
	}
	return srv
}

// Listen opens a new listener on TCP with specified address.
func (server *Server) Listen(addr string) (err error) {
	if server.listener == nil {
		server.listener, err = net.Listen("tcp", addr)
		if err != nil {
			return err
		}
	}
	return
}

// Accept starts accepting RCON connections from the listener.
func (server *Server) Accept(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	server.serve(ctx)
}

// Handler returns the server handler.
func (server *Server) Handler() Handler {
	ptr := server.handler.Load()
	if ptr == nil || *ptr == nil {
		return NopHandler{}
	}
	return *ptr
}

// Listener returns the server listener.
func (server *Server) Listener() net.Listener {
	return server.listener
}

// newContext returns a Context instance.
func (server *Server) newContext(conn *Conn) (*Context, error) {
	req := Request{
		Packet:     new(rcon.Packet),
		Server:     server,
		Connection: conn,
	}

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := req.Packet.ReadFrom(conn); err != nil {
		return nil, fmt.Errorf("read packet: %w", err)
	}

	return event.C(req), nil
}

// serve handles incoming requests until a stop signal is given with Close.
func (server *Server) serve(ctx context.Context) {
	server.wg.Add(1)
	defer server.wg.Done()

	go server.accept(ctx) // listener.Accept wouldn't block function
	for {
		select {
		case incoming, ok := <-server.incoming:
			if !ok {
				return
			}
			go server.handleConn(incoming, ctx)
		case <-ctx.Done():
			return
		case <-server.close:
			return
		}
	}
}

func (server *Server) accept(ctx context.Context) {
	if server.listener == nil {
		panic("listener is nil")
	}

	server.wg.Add(1)
	defer server.wg.Done()
	defer server.listener.Close() // nolint:errcheck

	for {
		select {
		case <-ctx.Done():
			return
		case <-server.close:
			return
		default:
		}

		conn, err := server.listener.Accept()
		if err != nil {
			return
		}
		if conn != nil {
			server.incoming <- conn
		}
	}
}

// handleConn handles incoming client conn.
func (server *Server) handleConn(conn net.Conn, ctx context.Context) {
	server.wg.Add(1)

	addr := conn.RemoteAddr()
	rConn := &Conn{Conn: conn}

	server.connMu.Lock()
	server.conn.Set(addr, rConn)
	server.connMu.Unlock()

	defer func() {
		// Closing the connection itself
		_ = conn.Close()
		// Deleting connection from the list
		server.connMu.Lock()
		server.conn.Delete(addr)
		server.connMu.Unlock()
		// Reducing wait group
		server.wg.Done()
	}()

	for {
		select {
		case <-server.close:
			return
		case <-ctx.Done():
			return
		default:
		}

		ctx, err := server.newContext(rConn)
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				// Failed to read the packet
				server.logger.Debug("failed to read request", "err", err, "addr", conn.RemoteAddr())
			}
			return
		}

		request := ctx.Val()
		// Handling the packet
		if err = server.handlePacket(request, rConn); err != nil {
			if request.Packet.Type == PacketTypeAuth && errors.Is(err, context.Canceled) {
				// Send failed authentication to the connection
				rConn.failAuthentication()
			}
			server.logger.Debug("handle packet", "addr", conn.RemoteAddr(), "err", err)
			return
		}
	}
}

// handlePacket handles incoming packet from the RCON connection.
func (server *Server) handlePacket(req Request, conn *Conn) error {
	body := req.Packet.Body()

	switch req.Packet.Type {
	case PacketTypeAuth:
		ctx := event.C(req)
		if server.Handler().HandleLoginRequest(ctx); ctx.Cancelled() {
			return context.Canceled
		}
		// Validating auth request
		validPass := bytes.Equal(server.password, []byte(body))
		if !validPass {
			// Sending fail response
			conn.failAuthentication()
			return fmt.Errorf("invalid password")
		}
		// Sending success response
		conn.succeedAuthentication(req.Packet.ID)
		conn.authenticated.Store(true)
	case PacketTypeExecCommand:
		if !conn.authenticated.Load() {
			return fmt.Errorf("sent execute command packet, but connection isn't authenticated")
		}
		ctx := event.C(req)
		if server.Handler().HandleMessage(ctx, body); ctx.Cancelled() {
			return context.Canceled
		}
	default:
		server.logger.Debug("unhandled packet", "type", req.Packet.Type, "id", req.Packet.ID, "addr", conn.RemoteAddr())
	}

	return nil
}

// Close shuts down the Server.
func (server *Server) Close() error {
	var err error
	server.once.Do(func() {
		close(server.close)
		// Closing listener to stop accepting incoming connections
		close(server.incoming)
		err = server.listener.Close()
		// Sending close signal

		// Clearing connections
		server.connMu.Lock()
		server.conn.Iterate(func(addr net.Addr, conn *Conn) bool {
			_ = conn.Close()
			server.conn.Delete(addr)
			return true
		})
		clear(server.conn)
		server.conn = nil // for gc
		server.connMu.Unlock()

		// Waiting all connections and listener to close
		server.wg.Wait()
	})
	return err
}
