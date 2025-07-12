package rcon

import (
	"github.com/df-mc/dragonfly/server/event"
	"github.com/gorcon/rcon"
)

// Request represents an RCON request from client to the server.
type Request struct {
	// Packet is the request itself.
	Packet *rcon.Packet
	// Server is server, that accepted this request.
	Server *Server
	// Connection is the underlying RCON connection of the request sender.
	Connection *Conn
}

// Context is cancellable context for the request.
type Context = event.Context[Request]

// Handler is handler of the main actions of Server.
type Handler interface {
	// HandleMessage called whether client sends message request to the server.
	HandleMessage(ctx *Context, body string)
	// HandleLoginRequest handles whether client sends login request.
	HandleLoginRequest(ctx *Context)
}

// NopHandler is no-operation implementation of the Handler.
type NopHandler struct{}

func (NopHandler) HandleMessage(*Context, string) {}
func (NopHandler) HandleLoginRequest(*Context)    {}
