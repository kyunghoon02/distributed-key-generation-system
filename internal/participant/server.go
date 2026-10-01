package participant

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/api"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/cryptoadapter"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/durable"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/protocol"
)

type Server struct {
	machine *protocol.Machine
	journal *durable.WAL
}

func NewServer(id string, adapter cryptoadapter.Adapter) *Server {
	return &Server{machine: protocol.NewMachine(id, adapter)}
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
	return &Server{machine: machine, journal: journal}, nil
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
	var request api.Request
	response := api.Response{OK: false}
	if err := json.NewDecoder(conn).Decode(&request); err != nil {
		response.Error = fmt.Sprintf("decode request: %v", err)
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
		if err := s.machine.ReceiveShare(request.Message); err != nil {
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
	case "status":
		response.OK = true
	default:
		response.Error = fmt.Sprintf("unknown operation %q", request.Operation)
	}
	response.Status = s.machine.Status()
	_ = json.NewEncoder(conn).Encode(response)
}
