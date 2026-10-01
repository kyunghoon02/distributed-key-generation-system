package participant

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/bytemare/frost"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/agentpayment"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/api"
)

type pendingPayment struct {
	digest       [32]byte
	amount       int64
	commitmentID uint64
}

func (s *RealServer) configureSigning(config api.RealSignConfig) error {
	if s.signer != nil || config.Threshold != 3 || len(config.Results) != 4 ||
		config.Policy.KeyID != s.keyID || config.Policy.KeyVersion != s.keyVersion {
		return errors.New("signing configuration already set or invalid")
	}
	if err := config.Policy.Validate(); err != nil {
		return err
	}
	signer, err := s.node.FrostSigner(config.Results, config.Threshold)
	if err != nil {
		return err
	}
	policy := config.Policy
	policy.AllowedMerchantIDs = slices.Clone(policy.AllowedMerchantIDs)
	s.signer, s.policy = signer, &policy
	return nil
}

func (s *RealServer) commitPayment(request agentpayment.Request) ([]byte, error) {
	if s.signer == nil || s.policy == nil {
		return nil, errors.New("signer not configured")
	}
	if _, exists := s.pending[request.RequestID]; exists || s.signed[request.RequestID] {
		return nil, errors.New("payment request ID already used")
	}
	if err := s.policy.Check(request, time.Now().UTC(), s.reserved); err != nil {
		return nil, err
	}
	digest, err := request.Digest()
	if err != nil {
		return nil, err
	}
	commitment := s.signer.Commit()
	s.pending[request.RequestID] = pendingPayment{digest: digest,
		amount: request.AmountMinor, commitmentID: commitment.CommitmentID}
	s.reserved += request.AmountMinor
	return commitment.Encode(), nil
}

func (s *RealServer) signPayment(input api.RealSignShare) ([]byte, error) {
	if s.signer == nil || s.policy == nil {
		return nil, errors.New("signer not configured")
	}
	pending, exists := s.pending[input.Request.RequestID]
	if !exists {
		return nil, errors.New("payment commitment missing or already consumed")
	}
	digest, err := input.Request.Digest()
	if err != nil || digest != pending.digest {
		s.burnPayment(input.Request.RequestID, pending)
		return nil, errors.New("payment request changed after commitment")
	}
	if err := s.policy.Check(input.Request, time.Now().UTC(), s.reserved-pending.amount); err != nil {
		s.burnPayment(input.Request.RequestID, pending)
		return nil, err
	}
	commitments, err := frost.DecodeList(input.Commitments)
	if err != nil {
		s.burnPayment(input.Request.RequestID, pending)
		return nil, fmt.Errorf("decode FROST commitments: %w", err)
	}
	if own := commitments.Get(s.signer.Identifier()); own == nil || own.CommitmentID != pending.commitmentID {
		s.burnPayment(input.Request.RequestID, pending)
		return nil, errors.New("own FROST commitment missing or changed")
	}
	message, err := input.Request.Canonical()
	if err != nil {
		s.burnPayment(input.Request.RequestID, pending)
		return nil, err
	}
	share, err := s.signer.Sign(message, commitments)
	s.burnPayment(input.Request.RequestID, pending)
	if err != nil {
		return nil, err
	}
	return share.Encode(), nil
}

func (s *RealServer) burnPayment(requestID string, pending pendingPayment) {
	s.signer.ClearNonceCommitment(pending.commitmentID)
	delete(s.pending, requestID)
	s.signed[requestID] = true
	// The local budget reservation remains consumed after any signing attempt.
	// This fails closed when the caller cannot prove whether a share escaped.
}
