package transport

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/api"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/protocol"
)

type Transport interface {
	Send(context.Context, string, protocol.Message) error
}

type TCP struct {
	Timeout   time.Duration
	TLSConfig *tls.Config
}

func (t TCP) Send(ctx context.Context, address string, message protocol.Message) error {
	_, err := t.Call(ctx, address, api.Request{Operation: "deliver", Message: message})
	return err
}

func (t TCP) Call(ctx context.Context, address string, request api.Request) (api.Response, error) {
	timeout := t.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	dialer := net.Dialer{Timeout: timeout}
	var conn net.Conn
	var err error
	if t.TLSConfig == nil {
		conn, err = dialer.DialContext(ctx, "tcp", address)
	} else {
		conn, err = (&tls.Dialer{NetDialer: &dialer, Config: t.TLSConfig}).DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return api.Response{}, fmt.Errorf("connect to participant at %s: %w", address, err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(timeout))
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return api.Response{}, fmt.Errorf("send request: %w", err)
	}
	var response api.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return api.Response{}, fmt.Errorf("read response: %w", err)
	}
	if !response.OK {
		return response, fmt.Errorf("participant request failed: %s", response.Error)
	}
	return response, nil
}
