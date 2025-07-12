package rcon

import (
	"github.com/gorcon/rcon"
	"net"
	"sync/atomic"
)

// Conn is implementation of the RCON connection.
type Conn struct {
	net.Conn
	authenticated atomic.Bool
}

// WritePacket writes RCON packet to the connection.
func (conn *Conn) WritePacket(pk *rcon.Packet) (int, error) {
	res, err := pk.WriteTo(conn.Conn)
	return int(res), err
}

// Response sends response packet to the connection with the specified body.
func (conn *Conn) Response(body string, packetID int32) (int, error) {
	return conn.WritePacket(rcon.NewPacket(
		PacketTypeResponseValue,
		packetID,
		body,
	))
}

func (conn *Conn) failAuthentication() {
	body := []byte{0x00}
	_, _ = conn.WritePacket(rcon.NewPacket(
		PacketTypeAuthResponse,
		-1, // when authentication request is invalid, -1 must be used as packet ID
		string(body),
	))
}

func (conn *Conn) succeedAuthentication(id int32) {
	// Writing empty response
	_, _ = conn.Response("", id)
	// Sending auth response
	_, _ = conn.WritePacket(rcon.NewPacket(
		PacketTypeAuthResponse,
		PacketIDAuth,
		"",
	))
}
