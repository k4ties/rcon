package rcon_test

import (
	"context"
	rrcon "github.com/gorcon/rcon"
	"github.com/k4ties/rcon"
	"testing"
	"time"
)

func TestListen(t *testing.T) {
	_ = listen(t, "secret").Close()
}

func TestAccept(t *testing.T) {
	srv := listen(t, "secret")

	ctx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()

	srv.Accept(ctx)
	_ = srv.Close()
}

func TestDial(t *testing.T) {
	const pass = "secret"
	srv := listen(t, pass, rcon.OptionHandler(testHandler{t: t}))

	go srv.Accept(nil)
	defer srv.Close()

	conn, err := rrcon.Dial(listenAddr, pass)
	if err != nil {
		t.Fatal(err)
	}

	res, err := conn.Execute("some message")
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("got response: %s", res)
}

const listenAddr = ":6060"

func listen(t *testing.T, pass string, opts ...rcon.ServerOption) *rcon.Server {
	srv := rcon.NewServer([]byte(pass), opts...)
	if err := srv.Listen(listenAddr); err != nil {
		t.Fatal(err)
	}
	return srv
}

type testHandler struct {
	rcon.NopHandler
	t *testing.T
}

func (h testHandler) HandleLoginRequest(ctx *rcon.Context) {
	h.t.Logf("login: %s", ctx.Val().Connection.RemoteAddr())
}

func (h testHandler) HandleMessage(ctx *rcon.Context, msg string) {
	req := ctx.Val()
	conn := req.Connection

	h.t.Logf("got message from %s: %s", conn.RemoteAddr(), msg)
	h.t.Logf("sending response")

	_, _ = conn.Response("some response", req.Packet.ID)
}
