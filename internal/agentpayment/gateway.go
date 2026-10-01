package agentpayment

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

type Receipt struct {
	PaymentID   string    `json:"payment_id"`
	RequestID   string    `json:"request_id"`
	MerchantID  string    `json:"merchant_id"`
	AmountMinor int64     `json:"amount_minor"`
	Currency    string    `json:"currency"`
	Status      string    `json:"status"`
	RecordedAt  time.Time `json:"recorded_at"`
}

type settledPayment struct {
	digest  [32]byte
	receipt Receipt
}

// SandboxGateway verifies a threshold authorization and records a simulated
// payment. It never calls a real card processor, wallet, or payment network.
type SandboxGateway struct {
	mu     sync.Mutex
	policy Policy
	public ed25519.PublicKey
	spent  int64
	paid   map[string]settledPayment
}

func NewSandboxGateway(policy Policy, public ed25519.PublicKey) (*SandboxGateway, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	if len(public) != ed25519.PublicKeySize {
		return nil, errors.New("invalid group public key")
	}
	return &SandboxGateway{policy: policy, public: bytes.Clone(public),
		paid: make(map[string]settledPayment)}, nil
}

func (g *SandboxGateway) Pay(authorization Authorization, now time.Time) (Receipt, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	request := authorization.Request
	message, err := request.Canonical()
	if err != nil {
		return Receipt{}, err
	}
	if !bytes.Equal(authorization.GroupPublic, g.public) ||
		!ed25519.Verify(g.public, message, authorization.Signature) {
		return Receipt{}, errors.New("invalid payment authorization signature")
	}
	digest, err := request.Digest()
	if err != nil {
		return Receipt{}, err
	}
	if prior, ok := g.paid[request.RequestID]; ok {
		if prior.digest != digest {
			return Receipt{}, errors.New("payment request ID reused with different contents")
		}
		return prior.receipt, nil
	}
	if err := g.policy.Check(request, now, g.spent); err != nil {
		return Receipt{}, err
	}
	receipt := Receipt{PaymentID: "sandbox-" + hex.EncodeToString(digest[:12]),
		RequestID: request.RequestID, MerchantID: request.MerchantID,
		AmountMinor: request.AmountMinor, Currency: request.Currency,
		Status: "simulated_paid", RecordedAt: now.UTC()}
	g.spent += request.AmountMinor
	g.paid[request.RequestID] = settledPayment{digest: digest, receipt: receipt}
	return receipt, nil
}
