package main

import (
	"context"
	"crypto/tls"
	"net"
	"os"
	"testing"
	"time"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/api"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/participant"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/transport"
)

func TestRealRPCRequiresTrustedController(t *testing.T) {
	credentials, err := newLocalRealTLSCredentials()
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(credentials.dir)
	config, err := loadRealServerTLSConfig(credentials.serverCert[0], credentials.serverKey[0], credentials.caCert)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server, err := participant.NewRealServer("p1", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve(tls.NewListener(listener, config)) }()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	request := api.Request{Operation: "real-status"}
	client := transport.TCP{Timeout: time.Second, TLSConfig: credentials.clientConfig("p1")}
	if _, err := client.Call(ctx, listener.Addr().String(), request); err != nil {
		t.Fatalf("trusted controller rejected: %v", err)
	}
	anonymous := credentials.clientConfig("p1").Clone()
	anonymous.Certificates = nil
	if _, err := (transport.TCP{Timeout: time.Second, TLSConfig: anonymous}).Call(ctx, listener.Addr().String(), request); err == nil {
		t.Fatal("anonymous controller was accepted")
	}
	wrongServer := credentials.clientConfig("p2")
	if _, err := (transport.TCP{Timeout: time.Second, TLSConfig: wrongServer}).Call(ctx, listener.Addr().String(), request); err == nil {
		t.Fatal("wrong server identity was accepted")
	}
}
