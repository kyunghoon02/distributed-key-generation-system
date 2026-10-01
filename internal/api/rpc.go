package api

import (
	"github.com/kyunghoon02/distributed-key-generation-system/internal/agentpayment"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/cryptoadapter"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/protocol"
)

type RealSignConfig struct {
	Results   []cryptoadapter.KyberPublicResult `json:"results"`
	Threshold int                               `json:"threshold"`
	Policy    agentpayment.Policy               `json:"policy"`
}

type RealSignShare struct {
	Request     agentpayment.Request `json:"request"`
	Commitments []byte               `json:"commitments"`
}

type RealConfig struct {
	Identities []cryptoadapter.KyberIdentity `json:"identities"`
	Threshold  int                           `json:"threshold"`
	Nonce      []byte                        `json:"nonce"`
	Peers      map[string]string             `json:"peers,omitempty"`
	KeyID      string                        `json:"key_id,omitempty"`
	KeyVersion uint64                        `json:"key_version,omitempty"`
}

type Request struct {
	Operation      string                     `json:"operation"`
	Config         protocol.Config            `json:"config,omitempty"`
	Message        protocol.Message           `json:"message,omitempty"`
	RealConfig     *RealConfig                `json:"real_config,omitempty"`
	RealPacket     *cryptoadapter.KyberPacket `json:"real_packet,omitempty"`
	RealSignConfig *RealSignConfig            `json:"real_sign_config,omitempty"`
	PaymentRequest *agentpayment.Request      `json:"payment_request,omitempty"`
	RealSignShare  *RealSignShare             `json:"real_sign_share,omitempty"`
}

type Response struct {
	OK             bool                             `json:"ok"`
	Error          string                           `json:"error,omitempty"`
	Outbound       []protocol.Message               `json:"outbound,omitempty"`
	Status         protocol.Status                  `json:"status,omitempty"`
	RealIdentity   *cryptoadapter.KyberIdentity     `json:"real_identity,omitempty"`
	RealPacket     *cryptoadapter.KyberPacket       `json:"real_packet,omitempty"`
	RealResult     *cryptoadapter.KyberPublicResult `json:"real_result,omitempty"`
	RealStage      string                           `json:"real_stage,omitempty"`
	RealDuplicate  bool                             `json:"real_duplicate,omitempty"`
	PeerRunning    bool                             `json:"peer_running,omitempty"`
	PeerError      string                           `json:"peer_error,omitempty"`
	SignCommitment []byte                           `json:"sign_commitment,omitempty"`
	SignShare      []byte                           `json:"sign_share,omitempty"`
}
