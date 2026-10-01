package participant

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/api"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/cryptoadapter"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/durable"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/protocol"
)

type Server struct {
	machine *protocol.Machine
	journal *durable.WAL
	metrics *metrics
	logger  *slog.Logger
}

func NewServer(id string, adapter cryptoadapter.Adapter) *Server {
	return newServer(protocol.NewMachine(id, adapter), nil)
}

func NewDurableServer(id string, adapter cryptoadapter.Adapter, stateFile string) (*Server, error) {
	journal, err := durable.Open(stateFile)
	if err != nil {
		return nil, err
	}
	machine, err := protocol.RecoverMachine(id, adapter, journal)
	if err != nil {
		_ = journal.Close()
		return nil, err
	}
	return newServer(machine, journal), nil
}

func newServer(machine *protocol.Machine, journal *durable.WAL) *Server {
	return &Server{
		machine: machine, journal: journal, metrics: newMetrics(machine),
		logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
	}
}

func (s *Server) SetLogger(logger *slog.Logger) {
	s.logger = logger
}

func (s *Server) MetricsHandler() http.Handler {
	return s.metrics.handler()
}

func (s *Server) Close() error {
	if s.journal != nil {
		return s.journal.Close()
	}
	return nil
}

func (s *Server) Serve(listener net.Listener) error {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	started := time.Now()
	var request api.Request
	response := api.Response{OK: false}
	if err := json.NewDecoder(conn).Decode(&request); err != nil {
		response.Error = fmt.Sprintf("decode request: %v", err)
		response.Status = s.machine.Status()
		s.record("decode", response, started)
		_ = json.NewEncoder(conn).Encode(response)
		return
	}

	switch request.Operation {
	case "begin":
		outbound, err := s.machine.Begin(request.Config)
		if err != nil {
			response.Error = err.Error()
		} else {
			response.Outbound = outbound
			response.OK = true
		}
	case "deliver":
		outcome, err := s.machine.ReceiveShareWithOutcome(request.Message)
		s.metrics.observeShare(outcome, err)
		if err != nil {
			response.Error = err.Error()
		} else {
			response.OK = true
		}
	case "finalize":
		if err := s.machine.Finalize(); err != nil {
			response.Error = err.Error()
		} else {
			response.OK = true
		}
	case "timeout":
		if err := s.machine.Timeout(); err != nil {
			response.Error = err.Error()
		} else {
			response.OK = true
		}
	case "status":
		response.OK = true
	default:
		response.Error = fmt.Sprintf("unknown operation %q", request.Operation)
	}
	response.Status = s.machine.Status()
	s.record(request.Operation, response, started)
	_ = json.NewEncoder(conn).Encode(response)
}

func (s *Server) record(operation string, response api.Response, started time.Time) {
	s.metrics.observeRequest(operation, response.OK, time.Since(started))
	s.logger.Info("participant_request",
		"operation", operation,
		"participant_id", response.Status.ParticipantID,
		"session_id", response.Status.SessionID,
		"phase", response.Status.Phase,
		"ok", response.OK,
		"error", response.Error,
	)
}
