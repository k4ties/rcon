package rcon

import (
	"log/slog"
	"net"
)

// ServerOption is option implementation for the Server. Options can only be
// applied before server started.
type ServerOption = func(*Server)

// OptionHandler is option, that allows users to set custom handlers to server.
func OptionHandler(handler Handler) ServerOption {
	return func(srv *Server) {
		if handler == nil {
			return
		}
		// Updating the logger
		srv.handler.Store(&handler)
	}
}

// OptionListener is option, that allows users to set custom listeners to the
// RCON server. If current listener isn't nil, it'll be closed.
func OptionListener(listener net.Listener) ServerOption {
	return func(srv *Server) {
		if listener == nil {
			return
		}
		if srv.listener != nil {
			// Closing current listener
			_ = srv.listener.Close()
		}
		// Updating listener
		srv.listener = listener
	}
}

// OptionLogger is option, that allows users to set custom loggers to the RCON
// server.
func OptionLogger(log *slog.Logger) ServerOption {
	return func(srv *Server) {
		if log == nil {
			return
		}
		// Updating the logger
		srv.logger = log
	}
}
