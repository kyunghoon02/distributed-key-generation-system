package cryptoadapter

// Adapter isolates runtime orchestration from protocol cryptography.
// Implementations in this package are not evidence of cryptographic security.
type Adapter interface {
	CreateContribution(sessionID, participantID string) (string, error)
	VerifyContribution(sessionID, participantID, contribution string) bool
}
