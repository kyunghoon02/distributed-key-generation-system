package cryptoadapter

import (
	"bytes"
	"testing"

	"github.com/bytemare/ecc"
	"github.com/bytemare/frost"
	"github.com/bytemare/secret-sharing/keys"
	"github.com/drand/kyber/share"
	"github.com/drand/kyber/share/dkg"
	"github.com/drand/kyber/sign/schnorr"
)

func TestKyberNormalCeremony(t *testing.T) {
	const count = 4
	participants := make([]*KyberParticipant, count)
	identities := make([]KyberIdentity, count)
	for i := range participants {
		var err error
		participants[i], err = NewKyberParticipant("p"+string(rune('1'+i)), uint32(i))
		if err != nil {
			t.Fatal(err)
		}
		identities[i], err = participants[i].Identity()
		if err != nil {
			t.Fatal(err)
		}
	}
	nonce := dkg.GetNonce()
	for _, participant := range participants {
		if err := participant.Configure(identities, 3, nonce); err != nil {
			t.Fatal(err)
		}
	}
	deals := make([]KyberPacket, count)
	for i, participant := range participants {
		var err error
		deals[i], err = participant.Deals()
		if err != nil {
			t.Fatal(err)
		}
	}
	for i, participant := range participants {
		for j, deal := range deals {
			if i == j {
				continue
			}
			if _, err := participant.Accept(deal); err != nil {
				t.Fatal(err)
			}
		}
	}
	responses := make([]*KyberPacket, count)
	for i, participant := range participants {
		var err error
		responses[i], err = participant.ProcessDeals()
		if err != nil {
			t.Fatal(err)
		}
	}
	for i, participant := range participants {
		for j, response := range responses {
			if i == j || response == nil {
				continue
			}
			if _, err := participant.Accept(*response); err != nil {
				t.Fatal(err)
			}
		}
	}
	var common []byte
	results := make([]KyberPublicResult, count)
	for i, participant := range participants {
		justification, err := participant.ProcessResponses()
		if err != nil {
			t.Fatal(err)
		}
		if justification != nil {
			t.Fatal("unexpected justification")
		}
		result, err := participant.PublicResult()
		if err != nil {
			t.Fatal(err)
		}
		if len(common) == 0 {
			common = result.GroupPublic
		} else if !bytes.Equal(common, result.GroupPublic) {
			t.Fatal("group keys disagree")
		}
		if len(result.PublicShare) == 0 || result.Qualified != count {
			t.Fatalf("invalid result: %+v", result)
		}
		results[i] = result
	}
	if _, err := participants[0].FrostSigner(results, 3); err != nil {
		t.Fatalf("import finalized DKG share: %v", err)
	}
	invalid := append([]KyberPublicResult(nil), results...)
	invalid[1].Index = count
	if _, err := participants[0].FrostSigner(invalid, 3); err == nil {
		t.Fatal("out-of-range signer index accepted")
	}
	invalid = append([]KyberPublicResult(nil), results...)
	invalid[0].GroupPublic, _ = participants[0].suite.Point().Base().MarshalBinary()
	for i := 1; i < count; i++ {
		invalid[i].GroupPublic = invalid[0].GroupPublic
	}
	if _, err := participants[0].FrostSigner(invalid, 3); err == nil {
		t.Fatal("group key differing from local DKG accepted")
	}
	// Test-only reconstruction checks that the finalized shares form a 3-of-4
	// key. Private shares never cross the process RPC in the real runner.
	shares := []*share.PriShare{
		participants[0].result.Key.Share,
		participants[1].result.Key.Share,
		participants[2].result.Key.Share,
	}
	secret, err := share.RecoverSecret(participants[0].suite, shares, 3, count)
	if err != nil {
		t.Fatal(err)
	}
	message := []byte("DKG share reconstruction test")
	signature, err := schnorr.Sign(participants[0].suite, secret, message)
	if err != nil {
		t.Fatal(err)
	}
	if err := schnorr.Verify(participants[0].suite, participants[0].result.Key.Public(), message, signature); err != nil {
		t.Fatalf("reconstructed key cannot sign for DKG group public key: %v", err)
	}
	if _, err := share.RecoverSecret(participants[0].suite, shares[:2], 3, count); err == nil {
		t.Fatal("two shares unexpectedly reconstructed a 3-of-4 key")
	}
	// The production bridge must import each DKG share locally. This test
	// verifies byte-level compatibility without reconstructing for signing.
	publicShares := make([]*keys.PublicKeyShare, count)
	frostShares := make([]*keys.KeyShare, count)
	for i, participant := range participants {
		secretBytes, err := participant.result.Key.Share.V.MarshalBinary()
		if err != nil {
			t.Fatal(err)
		}
		public, err := participant.PublicResult()
		if err != nil {
			t.Fatal(err)
		}
		frostShares[i], err = frost.NewKeyShare(frost.Ed25519, uint16(i+1), secretBytes, public.PublicShare, common)
		if err != nil {
			t.Fatalf("import DKG share %d: %v", i, err)
		}
		publicShares[i] = frostShares[i].PublicKeyShare()
	}
	group := ecc.Edwards25519Sha512.NewElement()
	if err := group.Decode(common); err != nil {
		t.Fatal(err)
	}
	configuration := &frost.Configuration{Ciphersuite: frost.Ed25519, Threshold: 3,
		MaxSigners: count, VerificationKey: group, SignerPublicKeyShares: publicShares}
	if err := configuration.Init(); err != nil {
		t.Fatal(err)
	}
	signers := make([]*frost.Signer, 3)
	commitments := make(frost.CommitmentList, 3)
	for i := range signers {
		signers[i], err = configuration.Signer(frostShares[i])
		if err != nil {
			t.Fatal(err)
		}
		commitments[i] = signers[i].Commit()
	}
	commitments.Sort()
	signatureShares := make([]*frost.SignatureShare, 3)
	for i, signer := range signers {
		signatureShares[i], err = signer.Sign(message, commitments)
		if err != nil {
			t.Fatal(err)
		}
	}
	thresholdSignature, err := configuration.AggregateSignatures(message, signatureShares, commitments, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := frost.VerifySignature(frost.Ed25519, message, thresholdSignature, group); err != nil {
		t.Fatal(err)
	}
}

func TestKyberRejectsTamperedSignedDeal(t *testing.T) {
	first, err := NewKyberParticipant("p1", 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewKyberParticipant("p2", 1)
	if err != nil {
		t.Fatal(err)
	}
	firstID, err := first.Identity()
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := second.Identity()
	if err != nil {
		t.Fatal(err)
	}
	nonce := dkg.GetNonce()
	for _, node := range []*KyberParticipant{first, second} {
		if err := node.Configure([]KyberIdentity{firstID, secondID}, 2, nonce); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := first.Deals(); err != nil {
		t.Fatal(err)
	}
	deal, err := second.Deals()
	if err != nil {
		t.Fatal(err)
	}
	tampered := deal
	tampered.Signature = bytes.Clone(deal.Signature)
	tampered.Signature[0] ^= 0xff
	if _, err := first.Accept(tampered); err == nil {
		t.Fatal("tampered signature accepted")
	}
	if first.Stage() != "COLLECT_DEALS" {
		t.Fatal("tampered packet changed phase")
	}
	duplicate, err := first.Accept(deal)
	if err != nil || duplicate {
		t.Fatalf("valid deal rejected or treated as duplicate: duplicate=%v err=%v", duplicate, err)
	}
	if err := first.Abort(); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Accept(deal); err == nil {
		t.Fatal("aborted ceremony accepted a packet")
	}
	if err := first.Timeout(); err == nil {
		t.Fatal("aborted ceremony changed terminal state")
	}
	unconfigured, err := NewKyberParticipant("p3", 2)
	if err != nil {
		t.Fatal(err)
	}
	thirdID, err := unconfigured.Identity()
	if err != nil {
		t.Fatal(err)
	}
	if err := unconfigured.Abort(); err != nil {
		t.Fatal(err)
	}
	if err := unconfigured.Configure([]KyberIdentity{firstID, secondID, thirdID}, 2, nonce); err == nil {
		t.Fatal("aborted ceremony was configured")
	}
}
