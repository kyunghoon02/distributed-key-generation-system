package protocol

// Message is the M0 wire model. M1 will add stable logical message identity
// and explicit duplicate/stale-message accounting.
type Message struct {
	SessionID string `json:"session_id"`
	Epoch     uint64 `json:"epoch"`
	Round     uint64 `json:"round"`
	Phase     Phase  `json:"phase"`
	From      string `json:"from"`
	To        string `json:"to"`
	Payload   string `json:"payload"`
}
