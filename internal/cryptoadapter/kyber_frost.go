package cryptoadapter

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/bytemare/ecc"
	"github.com/bytemare/frost"
	"github.com/bytemare/secret-sharing/keys"
	"github.com/drand/kyber/share"
)

// FrostConfiguration builds the public signing configuration from DKG results.
// A participant additionally verifies all public shares against its own DKG
// commitments before it accepts this configuration.
func FrostConfiguration(results []KyberPublicResult, threshold int) (*frost.Configuration, error) {
	if len(results) == 0 || threshold < 1 || threshold > len(results) || len(results) > 65535 {
		return nil, errors.New("invalid FROST roster")
	}
	group := ecc.Edwards25519Sha512.NewElement()
	if err := group.Decode(results[0].GroupPublic); err != nil {
		return nil, fmt.Errorf("decode DKG group key: %w", err)
	}
	publicShares := make([]*keys.PublicKeyShare, 0, len(results))
	seen := make(map[uint32]bool, len(results))
	for _, result := range results {
		if result.ParticipantID == "" || result.Index >= uint32(len(results)) || seen[result.Index] ||
			!bytes.Equal(result.GroupPublic, results[0].GroupPublic) {
			return nil, errors.New("inconsistent DKG public results")
		}
		seen[result.Index] = true
		public, err := frost.NewPublicKeyShare(frost.Ed25519, uint16(result.Index+1), result.PublicShare)
		if err != nil {
			return nil, fmt.Errorf("decode DKG public share: %w", err)
		}
		publicShares = append(publicShares, public)
	}
	configuration := &frost.Configuration{Ciphersuite: frost.Ed25519, Threshold: uint16(threshold),
		MaxSigners: uint16(len(results)), VerificationKey: group, SignerPublicKeyShares: publicShares}
	if err := configuration.Init(); err != nil {
		return nil, fmt.Errorf("validate FROST configuration: %w", err)
	}
	return configuration, nil
}

// FrostSigner imports this participant's DKG share within its process. The
// secret bytes are never returned over the participant RPC.
func (p *KyberParticipant) FrostSigner(results []KyberPublicResult, threshold int) (*frost.Signer, error) {
	if p.stage != "FINALIZE" || p.result == nil || len(results) != len(p.memberIDs) {
		return nil, errors.New("DKG must finalize before signing")
	}
	configuration, err := FrostConfiguration(results, threshold)
	if err != nil {
		return nil, err
	}
	public, err := p.PublicResult()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(configuration.VerificationKey.Encode(), public.GroupPublic) {
		return nil, errors.New("FROST group key differs from local DKG result")
	}
	poly := share.NewPubPoly(p.suite, p.suite.Point().Base(), p.result.Key.Commitments())
	for _, result := range results {
		if p.memberIDs[result.Index] != result.ParticipantID {
			return nil, errors.New("FROST roster differs from DKG roster")
		}
		expected, err := poly.Eval(int(result.Index)).V.MarshalBinary()
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(expected, result.PublicShare) {
			return nil, errors.New("FROST public share differs from DKG commitments")
		}
	}
	secret, err := p.result.Key.Share.V.MarshalBinary()
	if err != nil {
		return nil, err
	}
	defer clear(secret)
	keyShare, err := frost.NewKeyShare(frost.Ed25519, uint16(p.index+1), secret,
		public.PublicShare, public.GroupPublic)
	if err != nil {
		return nil, fmt.Errorf("import DKG share into FROST: %w", err)
	}
	return configuration.Signer(keyShare)
}
