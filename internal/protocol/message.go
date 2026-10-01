package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Message carries one protocol delivery. Its ID identifies the logical message
// independently of the payload or delivery attempt; it does not authenticate it.
type Message struct {
	MessageID string `json:"message_id"`
	SessionID string `json:"session_id"`
	Epoch     uint64 `json:"epoch"`
	Round     uint64 `json:"round"`
	Phase     Phase  `json:"phase"`
	From      string `json:"from"`
	To        string `json:"to"`
	Payload   string `json:"payload"`
}

// LogicalID is stable for the same ceremony, phase, sender, and recipient.
func (m Message) LogicalID() string {
	identity, _ := json.Marshal(struct {
		SessionID string `json:"session_id"`
		Epoch     uint64 `json:"epoch"`
		Round     uint64 `json:"round"`
		Phase     Phase  `json:"phase"`
		From      string `json:"from"`
		To        string `json:"to"`
	}{m.SessionID, m.Epoch, m.Round, m.Phase, m.From, m.To})
	sum := sha256.Sum256(identity)
	return hex.EncodeToString(sum[:])
}
