package api

import "github.com/kyunghoon02/distributed-key-generation-system/internal/protocol"

type Request struct {
	Operation string           `json:"operation"`
	Config    protocol.Config  `json:"config,omitempty"`
	Message   protocol.Message `json:"message,omitempty"`
}

type Response struct {
	OK       bool               `json:"ok"`
	Error    string             `json:"error,omitempty"`
	Outbound []protocol.Message `json:"outbound,omitempty"`
	Status   protocol.Status    `json:"status,omitempty"`
}
