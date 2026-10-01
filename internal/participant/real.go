package participant

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/api"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/cryptoadapter"
)

// RealServer serializes access to a memory-only Kyber DKG participant. Its
// final result API omits the private DKG share. In peer mode, only roster
// members may deliver packets; justification payloads may contain share material.
type RealServer struct {
	mu          sync.Mutex
	node        *cryptoadapter.KyberParticipant
	logger      *slog.Logger
	metrics     *realMetrics
	peerTLS     *tls.Config
	peers       map[string]string
	peerStarted bool
	peerRunning bool
	peerError   string
}

func NewRealServer(id string, index uint32, logger *slog.Logger) (*RealServer, error) {
	node, err := cryptoadapter.NewKyberParticipant(id, index)
	if err != nil {
		return nil, err
	}
	server := &RealServer{node: node, logger: logger}
	server.metrics = newRealMetrics(server)
	return server, nil
}

func (s *RealServer) MetricsHandler() http.Handler { return s.metrics.handler() }

// EnablePeerMode permits authenticated roster members to deliver DKG packets.
// Controller operations remain available only to the controller certificate.
func (s *RealServer) EnablePeerMode(clientTLS *tls.Config) {
	s.peerTLS = clientTLS
}

func (s *RealServer) Serve(listener net.Listener) error {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		go s.handle(conn)
	}
}

func (s *RealServer) handle(conn net.Conn) {
	defer conn.Close()
	started := time.Now()
	var caller string
	if s.peerTLS != nil {
		secure, ok := conn.(*tls.Conn)
		if !ok || secure.Handshake() != nil {
			return
		}
		certs := secure.ConnectionState().PeerCertificates
		if len(certs) == 0 {
			return
		}
		caller = certs[0].Subject.CommonName
	}
	var request api.Request
	if err := json.NewDecoder(io.LimitReader(conn, 1<<20)).Decode(&request); err != nil {
		s.metrics.observeRequest("decode", false, time.Since(started))
		_ = json.NewEncoder(conn).Encode(api.Response{Error: fmt.Sprintf("decode request: %v", err)})
		return
	}
	s.mu.Lock()
	var response api.Response
	var err error
	if s.peerTLS != nil && caller != "dkg-controller" {
		if request.Operation != "real-accept" || request.RealPacket == nil ||
			request.RealPacket.From != caller || s.peers[caller] == "" {
			err = errors.New("peer may only deliver its own DKG packet")
		} else {
			response, err = s.dispatch(request)
		}
	} else if s.peerTLS != nil && (request.Operation == "real-accept" || request.Operation == "real-deals" ||
		request.Operation == "real-process-deals" || request.Operation == "real-process-responses" ||
		request.Operation == "real-process-justifications") {
		err = errors.New("controller cannot drive peer DKG packet phases")
	} else {
		response, err = s.dispatch(request)
	}
	response.RealStage = s.node.Stage()
	response.PeerRunning = s.peerRunning
	response.PeerError = s.peerError
	s.mu.Unlock()
	s.metrics.observeRequest(request.Operation, err == nil, time.Since(started))
	if request.Operation == "real-accept" {
		s.metrics.observePacket(response.RealDuplicate, err)
	}
	response.OK = err == nil
	if err != nil {
		response.Error = err.Error()
	}
	if s.logger != nil {
		s.logger.Info("real_dkg_request", "operation", request.Operation,
			"phase", response.RealStage, "ok", response.OK, "error", response.Error)
	}
	_ = json.NewEncoder(conn).Encode(response)
}

func (s *RealServer) dispatch(request api.Request) (api.Response, error) {
	var response api.Response
	switch request.Operation {
	case "real-identity":
		identity, err := s.node.Identity()
		response.RealIdentity = &identity
		return response, err
	case "real-configure":
		if request.RealConfig == nil {
			return response, errors.New("missing real config")
		}
		config := request.RealConfig
		if s.peerTLS != nil {
			if err := s.validatePeers(config); err != nil {
				return response, err
			}
		}
		if err := s.node.Configure(config.Identities, config.Threshold, config.Nonce); err != nil {
			return response, err
		}
		if s.peerTLS != nil {
			s.peers = config.Peers
		}
		return response, nil
	case "real-p2p-start":
		if s.peerTLS == nil || len(s.peers) == 0 || s.peerStarted || s.node.Stage() != "DEAL" {
			return response, errors.New("peer DKG is not ready or already started")
		}
		s.peerStarted, s.peerRunning = true, true
		go s.runPeerCeremony()
		return response, nil
	case "real-deals":
		packet, err := s.node.Deals()
		response.RealPacket = &packet
		return response, err
	case "real-accept":
		if request.RealPacket == nil {
			return response, errors.New("missing real packet")
		}
		duplicate, err := s.node.Accept(*request.RealPacket)
		response.RealDuplicate = duplicate
		return response, err
	case "real-process-deals":
		packet, err := s.node.ProcessDeals()
		response.RealPacket = packet
		return response, err
	case "real-process-responses":
		packet, err := s.node.ProcessResponses()
		response.RealPacket = packet
		return response, err
	case "real-process-justifications":
		return response, s.node.ProcessJustifications()
	case "real-result":
		result, err := s.node.PublicResult()
		response.RealResult = &result
		return response, err
	case "real-timeout":
		return response, s.node.Timeout()
	case "real-abort":
		return response, s.node.Abort()
	case "real-status":
		return response, nil
	default:
		return response, fmt.Errorf("unknown real DKG operation %q", request.Operation)
	}
}
