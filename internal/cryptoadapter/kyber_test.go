package cryptoadapter

import (
	"bytes"
	"testing"

	"github.com/drand/kyber/share/dkg"
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
	for _, participant := range participants {
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
