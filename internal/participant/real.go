package participant

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/api"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/cryptoadapter"
)

// RealServer serializes access to a memory-only Kyber DKG participant. It
// never exposes private scalar values or the final private signing share.
type RealServer struct {
	mu     sync.Mutex
	node   *cryptoadapter.KyberParticipant
	logger *slog.Logger
}

func NewRealServer(id string, index uint32, logger *slog.Logger) (*RealServer, error) {
	node, err := cryptoadapter.NewKyberParticipant(id, index)
	if err != nil {
		return nil, err
	}
	return &RealServer{node: node, logger: logger}, nil
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
	var request api.Request
	if err := json.NewDecoder(io.LimitReader(conn, 1<<20)).Decode(&request); err != nil {
		_ = json.NewEncoder(conn).Encode(api.Response{Error: fmt.Sprintf("decode request: %v", err)})
		return
	}
	s.mu.Lock()
	response, err := s.dispatch(request)
	response.RealStage = s.node.Stage()
	s.mu.Unlock()
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
		return response, s.node.Configure(config.Identities, config.Threshold, config.Nonce)
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
