package protocol

type Config struct {
	SessionID    string   `json:"session_id"`
	Epoch        uint64   `json:"epoch"`
	Round        uint64   `json:"round"`
	Threshold    int      `json:"threshold"`
	Participants []string `json:"participants"`
}
