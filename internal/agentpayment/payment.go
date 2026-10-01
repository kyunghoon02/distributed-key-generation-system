package agentpayment

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const authorizationDomain = "dkgctl.agent-payment-authorization.v1\x00"

// Request is a single sandbox payment authorization, not a payment-rail
// transaction. AmountMinor is an integer in the currency's minor units.
type Request struct {
	RequestID   string    `json:"request_id"`
	AgentID     string    `json:"agent_id"`
	MerchantID  string    `json:"merchant_id"`
	BudgetID    string    `json:"budget_id"`
	KeyID       string    `json:"key_id"`
	KeyVersion  uint64    `json:"key_version"`
	AmountMinor int64     `json:"amount_minor"`
	Currency    string    `json:"currency"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// Policy is provisioned to each signer before authorization. Independent
// signers enforce it locally; the controller's decision alone is insufficient.
type Policy struct {
	BudgetID           string    `json:"budget_id"`
	AgentID            string    `json:"agent_id"`
	KeyID              string    `json:"key_id"`
	KeyVersion         uint64    `json:"key_version"`
	Currency           string    `json:"currency"`
	AllowedMerchantIDs []string  `json:"allowed_merchant_ids"`
	MaxPerPaymentMinor int64     `json:"max_per_payment_minor"`
	MaxTotalMinor      int64     `json:"max_total_minor"`
	ValidUntil         time.Time `json:"valid_until"`
}

func validID(value string) bool {
	return value != "" && len(value) <= 128 && strings.TrimSpace(value) == value &&
		!strings.ContainsAny(value, "\x00\n\r\t")
}

func (p Policy) Validate() error {
	if !validID(p.BudgetID) || !validID(p.AgentID) || !validID(p.KeyID) || p.KeyVersion == 0 ||
		p.Currency == "" || p.MaxPerPaymentMinor <= 0 || p.MaxTotalMinor < p.MaxPerPaymentMinor ||
		p.ValidUntil.IsZero() || len(p.AllowedMerchantIDs) == 0 {
		return errors.New("invalid payment policy")
	}
	for _, merchant := range p.AllowedMerchantIDs {
		if !validID(merchant) {
			return errors.New("invalid merchant in payment policy")
		}
	}
	return nil
}

func (p Policy) Check(r Request, now time.Time, spent int64) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if !validID(r.RequestID) || !validID(r.AgentID) || !validID(r.MerchantID) || !validID(r.BudgetID) ||
		!validID(r.KeyID) || r.KeyVersion == 0 || r.AmountMinor <= 0 || r.ExpiresAt.IsZero() {
		return errors.New("invalid payment request")
	}
	if r.AgentID != p.AgentID || r.BudgetID != p.BudgetID || r.KeyID != p.KeyID ||
		r.KeyVersion != p.KeyVersion || r.Currency != p.Currency {
		return errors.New("payment request does not match policy")
	}
	if !now.Before(r.ExpiresAt) || !now.Before(p.ValidUntil) || r.ExpiresAt.After(p.ValidUntil) {
		return errors.New("payment request or policy expired")
	}
	allowed := false
	for _, merchant := range p.AllowedMerchantIDs {
		if r.MerchantID == merchant {
			allowed = true
			break
		}
	}
	if !allowed {
		return errors.New("merchant not allowed")
	}
	if r.AmountMinor > p.MaxPerPaymentMinor || spent < 0 || spent > p.MaxTotalMinor-r.AmountMinor {
		return errors.New("payment budget exceeded")
	}
	return nil
}

// Canonical returns a versioned, deterministic signing input. It is never
// assembled from an unstructured payload string.
func (r Request) Canonical() ([]byte, error) {
	if r.ExpiresAt.IsZero() {
		return nil, errors.New("missing payment expiry")
	}
	r.ExpiresAt = r.ExpiresAt.UTC()
	encoded, err := json.Marshal(r)
	if err != nil {
		return nil, fmt.Errorf("encode payment request: %w", err)
	}
	return append([]byte(authorizationDomain), encoded...), nil
}

func (r Request) Digest() ([32]byte, error) {
	encoded, err := r.Canonical()
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(encoded), nil
}

type Authorization struct {
	Request      Request   `json:"request"`
	Signature    []byte    `json:"signature"`
	GroupPublic  []byte    `json:"group_public"`
	SignerIDs    []string  `json:"signer_ids"`
	AuthorizedAt time.Time `json:"authorized_at"`
}
