package api

import (
	"github.com/kyunghoon02/distributed-key-generation-system/internal/cryptoadapter"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/protocol"
)

type RealConfig struct {
	Identities []cryptoadapter.KyberIdentity `json:"identities"`
	Threshold  int                           `json:"threshold"`
	Nonce      []byte                        `json:"nonce"`
}

type Request struct {
	Operation  string                     `json:"operation"`
	Config     protocol.Config            `json:"config,omitempty"`
	Message    protocol.Message           `json:"message,omitempty"`
	RealConfig *RealConfig                `json:"real_config,omitempty"`
	RealPacket *cryptoadapter.KyberPacket `json:"real_packet,omitempty"`
}

type Response struct {
	OK            bool                             `json:"ok"`
	Error         string                           `json:"error,omitempty"`
	Outbound      []protocol.Message               `json:"outbound,omitempty"`
	Status        protocol.Status                  `json:"status,omitempty"`
	RealIdentity  *cryptoadapter.KyberIdentity     `json:"real_identity,omitempty"`
	RealPacket    *cryptoadapter.KyberPacket       `json:"real_packet,omitempty"`
	RealResult    *cryptoadapter.KyberPublicResult `json:"real_result,omitempty"`
	RealStage     string                           `json:"real_stage,omitempty"`
	RealDuplicate bool                             `json:"real_duplicate,omitempty"`
}
