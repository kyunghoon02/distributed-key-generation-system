package main

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"log/slog"
	"net"
	"net/http"
	"os"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/participant"
)

func runRealParticipant(args []string) error {
	flags := flag.NewFlagSet("real-participant", flag.ContinueOnError)
	id := flags.String("id", "", "participant ID")
	index := flags.Uint("index", 0, "participant index")
	address := flags.String("listen", "127.0.0.1:0", "TCP listen address")
	metricsAddress := flags.String("metrics-listen", "", "optional loopback Prometheus HTTP listen address")
	tlsCert := flags.String("tls-cert", "", "server certificate PEM")
	tlsKey := flags.String("tls-key", "", "server private key PEM")
	clientCA := flags.String("tls-client-ca", "", "trusted controller CA PEM")
	peerMode := flags.Bool("peer-mode", false, "permit authenticated roster peers to deliver DKG packets")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *id == "" || *index > uint(^uint32(0)) {
		return errors.New("valid -id and -index are required")
	}
	if *tlsCert == "" || *tlsKey == "" || *clientCA == "" {
		return errors.New("real DKG RPC requires -tls-cert, -tls-key, and -tls-client-ca")
	}
	if *metricsAddress != "" {
		if err := requireLoopback(*metricsAddress); err != nil {
			return err
		}
	}
	serverTLS, err := loadRealServerTLSConfigWithPeers(*tlsCert, *tlsKey, *clientCA, *peerMode)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", *address)
	if err != nil {
		return err
	}
	defer listener.Close()
	listener = tls.NewListener(listener, serverTLS)
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	server, err := participant.NewRealServer(*id, uint32(*index), logger)
	if err != nil {
		return err
	}
	if *peerMode {
		clientTLS := &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: serverTLS.ClientCAs,
			Certificates: serverTLS.Certificates}
		server.EnablePeerMode(clientTLS)
	}
	if *metricsAddress != "" {
		metricsListener, err := net.Listen("tcp", *metricsAddress)
		if err != nil {
			return err
		}
		defer metricsListener.Close()
		mux := http.NewServeMux()
		mux.Handle("/metrics", server.MetricsHandler())
		go func() { _ = http.Serve(metricsListener, mux) }()
	}
	return server.Serve(listener)
}

func loadRealServerTLSConfig(certPath, keyPath, caPath string) (*tls.Config, error) {
	return loadRealServerTLSConfigWithPeers(certPath, keyPath, caPath, false)
}

func loadRealServerTLSConfigWithPeers(certPath, keyPath, caPath string, peerMode bool) (*tls.Config, error) {
	certificate, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return nil, err
	}
	clientRoots := x509.NewCertPool()
	if !clientRoots.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("invalid controller CA PEM")
	}
	return &tls.Config{MinVersion: tls.VersionTLS13,
		Certificates: []tls.Certificate{certificate}, ClientCAs: clientRoots,
		ClientAuth: tls.RequireAndVerifyClientCert,
		VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 {
				return errors.New("missing DKG client identity")
			}
			name := state.PeerCertificates[0].Subject.CommonName
			if name != "dkg-controller" && (!peerMode || (name != "p1" && name != "p2" && name != "p3" && name != "p4")) {
				return errors.New("untrusted DKG controller identity")
			}
			return nil
		},
	}, nil
}

func requireLoopback(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("real DKG endpoints must listen on loopback")
	}
	return nil
}
