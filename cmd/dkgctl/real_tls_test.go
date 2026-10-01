package main

import (
	"context"
	"crypto/tls"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/drand/kyber/share/dkg"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/api"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/cryptoadapter"
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

func TestPeerModeSeparatesControllerAndPeerRPC(t *testing.T) {
	credentials, err := newLocalRealTLSCredentials()
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(credentials.dir)
	config, err := loadRealServerTLSConfigWithPeers(credentials.serverCert[0], credentials.serverKey[0], credentials.caCert, true)
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
	server.EnablePeerMode(&tls.Config{MinVersion: tls.VersionTLS13, RootCAs: credentials.roots,
		Certificates: config.Certificates})
	go func() { _ = server.Serve(tls.NewListener(listener, config)) }()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	controller := transport.TCP{Timeout: time.Second, TLSConfig: credentials.clientConfig("p1")}
	if _, err := controller.Call(ctx, listener.Addr().String(), api.Request{Operation: "real-status"}); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"real-accept", "real-deals", "real-process-deals", "real-process-responses", "real-process-justifications"} {
		if _, err := controller.Call(ctx, listener.Addr().String(), api.Request{Operation: operation}); err == nil {
			t.Fatalf("controller could drive peer packet phase via %s", operation)
		}
	}
	peerCert, err := tls.LoadX509KeyPair(credentials.serverCert[1], credentials.serverKey[1])
	if err != nil {
		t.Fatal(err)
	}
	peerTLS := credentials.clientConfig("p1")
	peerTLS.Certificates = []tls.Certificate{peerCert}
	peer := transport.TCP{Timeout: time.Second, TLSConfig: peerTLS}
	if _, err := peer.Call(ctx, listener.Addr().String(), api.Request{Operation: "real-status"}); err == nil {
		t.Fatal("peer accessed controller status RPC")
	}
	identities := make([]cryptoadapter.KyberIdentity, 4)
	self, err := controller.Call(ctx, listener.Addr().String(), api.Request{Operation: "real-identity"})
	if err != nil {
		t.Fatal(err)
	}
	identities[0] = *self.RealIdentity
	for i := 1; i < 4; i++ {
		node, err := cryptoadapter.NewKyberParticipant([]string{"p2", "p3", "p4"}[i-1], uint32(i))
		if err != nil {
			t.Fatal(err)
		}
		identities[i], err = node.Identity()
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = controller.Call(ctx, listener.Addr().String(), api.Request{Operation: "real-configure",
		RealConfig: &api.RealConfig{Identities: identities, Threshold: 3, Nonce: dkg.GetNonce(),
			KeyID: "test-key", KeyVersion: 1,
			Peers: map[string]string{"p2": "127.0.0.1:10002", "p3": "127.0.0.1:10003", "p4": "127.0.0.1:10004"}}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := peer.Call(ctx, listener.Addr().String(), api.Request{Operation: "real-accept",
		RealPacket: &cryptoadapter.KyberPacket{From: "p3"}})
	if err == nil || !strings.Contains(response.Error, "peer may only deliver its own") {
		t.Fatalf("peer certificate could impersonate packet sender: %+v, %v", response, err)
	}
}
