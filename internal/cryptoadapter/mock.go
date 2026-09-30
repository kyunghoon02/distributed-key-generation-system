package cryptoadapter

import "fmt"

// Mock produces deterministic test data. It does not create keys, shares, or
// cryptographically secure material.
type Mock struct{}

func (Mock) CreateContribution(sessionID, participantID string) (string, error) {
	return fmt.Sprintf("mock-contribution:%s:%s", sessionID, participantID), nil
}

func (Mock) VerifyContribution(sessionID, participantID, contribution string) bool {
	want := fmt.Sprintf("mock-contribution:%s:%s", sessionID, participantID)
	return contribution == want
}
