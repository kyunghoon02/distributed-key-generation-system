package cryptoadapter

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/drand/kyber"
	"github.com/drand/kyber/group/edwards25519"
	"github.com/drand/kyber/share"
	"github.com/drand/kyber/share/dkg"
	"github.com/drand/kyber/sign/schnorr"
	"github.com/drand/kyber/util/random"
)

// KyberParticipant adapts the drand Kyber Pedersen DKG to the runtime's
// packet delivery model. A participant owns its long-term key and secret share;
// callers receive signed packets with encrypted deal shares and public results.
// This adapter is memory-only: its internal DKG state cannot be WAL replayed.
type KyberParticipant struct {
	id             string
	index          uint32
	suite          *edwards25519.SuiteEd25519
	private        kyber.Scalar
	config         *dkg.Config
	engine         *dkg.DistKeyGenerator
	memberIDs      map[uint32]string
	deals          map[uint32]*dkg.DealBundle
	responses      map[uint32]*dkg.ResponseBundle
	justifications map[uint32]*dkg.JustificationBundle
	seen           map[string][]byte
	result         *dkg.Result
	stage          string
}

var ErrStaleSession = errors.New("stale DKG session")

type KyberIdentity struct {
	ID     string
	Index  uint32
	Public []byte
}

type KyberPacket struct {
	Kind           string               `json:"kind"`
	From           string               `json:"from"`
	Index          uint32               `json:"index"`
	SessionID      []byte               `json:"session_id"`
	Deals          []dkg.Deal           `json:"deals,omitempty"`
	Public         [][]byte             `json:"public,omitempty"`
	Responses      []dkg.Response       `json:"responses,omitempty"`
	Justifications []KyberJustification `json:"justifications,omitempty"`
	Signature      []byte               `json:"signature"`
}

type KyberJustification struct {
	ShareIndex uint32 `json:"share_index"`
	Share      []byte `json:"share"`
}

type KyberPublicResult struct {
	ParticipantID string `json:"participant_id"`
	GroupPublic   []byte `json:"group_public"`
	PublicShare   []byte `json:"public_share"`
	Qualified     int    `json:"qualified"`
}

func NewKyberParticipant(id string, index uint32) (*KyberParticipant, error) {
	if id == "" {
		return nil, errors.New("empty participant ID")
	}
	suite := edwards25519.NewBlakeSHA256Ed25519()
	private := suite.Scalar().Pick(random.New(rand.Reader))
	return &KyberParticipant{id: id, index: index, suite: suite, private: private,
		deals: make(map[uint32]*dkg.DealBundle), responses: make(map[uint32]*dkg.ResponseBundle),
		justifications: make(map[uint32]*dkg.JustificationBundle), seen: make(map[string][]byte), stage: "INIT"}, nil
}

func (p *KyberParticipant) Identity() (KyberIdentity, error) {
	public, err := p.suite.Point().Mul(p.private, nil).MarshalBinary()
	return KyberIdentity{ID: p.id, Index: p.index, Public: public}, err
}

func (p *KyberParticipant) Configure(identities []KyberIdentity, threshold int, nonce []byte) error {
	if p.stage != "INIT" || p.engine != nil || len(nonce) != dkg.NonceLength || threshold < 1 || threshold > len(identities) {
		return errors.New("invalid or repeated DKG configuration")
	}
	nodes := make([]dkg.Node, 0, len(identities))
	memberIDs := make(map[uint32]string, len(identities))
	ids := make(map[string]bool, len(identities))
	indices := make(map[uint32]bool, len(identities))
	self := false
	for _, identity := range identities {
		if identity.ID == "" || ids[identity.ID] || indices[identity.Index] {
			return errors.New("duplicate or empty DKG identity")
		}
		ids[identity.ID], indices[identity.Index] = true, true
		memberIDs[identity.Index] = identity.ID
		point := p.suite.Point()
		if err := point.UnmarshalBinary(identity.Public); err != nil {
			return fmt.Errorf("decode public identity: %w", err)
		}
		if identity.ID == p.id {
			own, err := p.Identity()
			if err != nil || identity.Index != p.index || !bytes.Equal(identity.Public, own.Public) {
				return errors.New("own identity mismatch")
			}
			self = true
		}
		nodes = append(nodes, dkg.Node{Index: identity.Index, Public: point})
	}
	if !self {
		return errors.New("participant absent from DKG group")
	}
	config := &dkg.Config{Suite: p.suite, Longterm: p.private, NewNodes: nodes,
		Threshold: threshold, Nonce: bytes.Clone(nonce), Auth: schnorr.NewScheme(p.suite), FastSync: true}
	engine, err := dkg.NewDistKeyHandler(config)
	if err != nil {
		return err
	}
	p.config, p.engine, p.memberIDs, p.stage = config, engine, memberIDs, "DEAL"
	return nil
}

func (p *KyberParticipant) Deals() (KyberPacket, error) {
	if p.stage != "DEAL" {
		return KyberPacket{}, errors.New("deals in wrong phase")
	}
	bundle, err := p.engine.Deals()
	if err != nil {
		return KyberPacket{}, err
	}
	packet := KyberPacket{Kind: "deal", From: p.id, Index: bundle.DealerIndex,
		SessionID: bytes.Clone(bundle.SessionID), Deals: bundle.Deals, Signature: bytes.Clone(bundle.Signature)}
	for _, point := range bundle.Public {
		encoded, err := point.MarshalBinary()
		if err != nil {
			return KyberPacket{}, err
		}
		packet.Public = append(packet.Public, encoded)
	}
	p.stage = "COLLECT_DEALS"
	return packet, nil
}

func (p *KyberParticipant) Accept(packet KyberPacket) (bool, error) {
	if p.engine == nil {
		return false, errors.New("DKG not configured")
	}
	if p.stage == "TIMED_OUT" || p.stage == "ABORTED" {
		return false, errors.New("DKG already terminal")
	}
	if !bytes.Equal(packet.SessionID, p.config.Nonce) {
		return false, ErrStaleSession
	}
	if packet.Index == p.index || packet.From == p.id {
		return false, errors.New("self-delivery is invalid")
	}
	memberID, known := p.memberIDs[packet.Index]
	if !known {
		return false, errors.New("unknown DKG sender")
	}
	if packet.From != memberID {
		return false, errors.New("DKG sender/index mismatch")
	}
	var signed dkg.Packet
	switch packet.Kind {
	case "deal":
		bundle := &dkg.DealBundle{DealerIndex: packet.Index, Deals: packet.Deals,
			SessionID: packet.SessionID, Signature: packet.Signature}
		for _, raw := range packet.Public {
			point := p.suite.Point()
			if err := point.UnmarshalBinary(raw); err != nil {
				return false, err
			}
			bundle.Public = append(bundle.Public, point)
		}
		signed = bundle
	case "response":
		signed = &dkg.ResponseBundle{ShareIndex: packet.Index, Responses: packet.Responses,
			SessionID: packet.SessionID, Signature: packet.Signature}
	case "justification":
		bundle := &dkg.JustificationBundle{DealerIndex: packet.Index,
			SessionID: packet.SessionID, Signature: packet.Signature}
		for _, item := range packet.Justifications {
			scalar := p.suite.Scalar()
			if err := scalar.UnmarshalBinary(item.Share); err != nil {
				return false, err
			}
			bundle.Justifications = append(bundle.Justifications, dkg.Justification{ShareIndex: item.ShareIndex, Share: scalar})
		}
		signed = bundle
	default:
		return false, errors.New("unknown DKG packet kind")
	}
	if err := dkg.VerifyPacketSignature(p.config, signed); err != nil {
		return false, fmt.Errorf("invalid DKG signature: %w", err)
	}
	key := fmt.Sprintf("%s:%d", packet.Kind, packet.Index)
	hash := signed.Hash()
	if previous, ok := p.seen[key]; ok {
		if bytes.Equal(previous, hash) {
			return true, nil
		}
		return false, errors.New("conflicting DKG packet")
	}
	expectedStage := map[string]string{"deal": "COLLECT_DEALS", "response": "COLLECT_RESPONSES", "justification": "COLLECT_JUSTIFICATIONS"}[packet.Kind]
	if p.stage != expectedStage {
		return false, errors.New("DKG packet in wrong phase")
	}
	p.seen[key] = bytes.Clone(hash)
	switch bundle := signed.(type) {
	case *dkg.DealBundle:
		p.deals[packet.Index] = bundle
	case *dkg.ResponseBundle:
		p.responses[packet.Index] = bundle
	case *dkg.JustificationBundle:
		p.justifications[packet.Index] = bundle
	}
	return false, nil
}

func (p *KyberParticipant) ProcessDeals() (*KyberPacket, error) {
	if p.stage != "COLLECT_DEALS" {
		return nil, errors.New("process deals in wrong phase")
	}
	var deals []*dkg.DealBundle
	for _, bundle := range p.deals {
		deals = append(deals, bundle)
	}
	response, err := p.engine.ProcessDeals(deals)
	if err != nil {
		return nil, err
	}
	p.stage = "COLLECT_RESPONSES"
	if response == nil {
		return nil, nil
	}
	return &KyberPacket{Kind: "response", From: p.id, Index: response.ShareIndex,
		SessionID: bytes.Clone(response.SessionID), Responses: response.Responses,
		Signature: bytes.Clone(response.Signature)}, nil
}

func (p *KyberParticipant) ProcessResponses() (*KyberPacket, error) {
	if p.stage != "COLLECT_RESPONSES" {
		return nil, errors.New("process responses in wrong phase")
	}
	var responses []*dkg.ResponseBundle
	for _, bundle := range p.responses {
		responses = append(responses, bundle)
	}
	result, justification, err := p.engine.ProcessResponses(responses)
	if err != nil {
		return nil, err
	}
	if result != nil {
		p.result, p.stage = result, "FINALIZE"
		return nil, nil
	}
	p.stage = "COLLECT_JUSTIFICATIONS"
	if justification == nil {
		return nil, nil
	}
	packet := &KyberPacket{Kind: "justification", From: p.id, Index: justification.DealerIndex,
		SessionID: bytes.Clone(justification.SessionID), Signature: bytes.Clone(justification.Signature)}
	for _, item := range justification.Justifications {
		raw, err := item.Share.MarshalBinary()
		if err != nil {
			return nil, err
		}
		packet.Justifications = append(packet.Justifications, KyberJustification{ShareIndex: item.ShareIndex, Share: raw})
	}
	return packet, nil
}

func (p *KyberParticipant) ProcessJustifications() error {
	if p.stage != "COLLECT_JUSTIFICATIONS" {
		return errors.New("process justifications in wrong phase")
	}
	var bundles []*dkg.JustificationBundle
	for _, bundle := range p.justifications {
		bundles = append(bundles, bundle)
	}
	result, err := p.engine.ProcessJustifications(bundles)
	if err != nil {
		return err
	}
	if result == nil {
		return errors.New("DKG finished without a result")
	}
	p.result, p.stage = result, "FINALIZE"
	return nil
}

func (p *KyberParticipant) Stage() string { return p.stage }

func (p *KyberParticipant) Timeout() error {
	if p.stage == "TIMED_OUT" {
		return nil
	}
	if p.stage == "FINALIZE" || p.stage == "ABORTED" {
		return errors.New("cannot time out a terminal DKG")
	}
	p.stage = "TIMED_OUT"
	return nil
}

func (p *KyberParticipant) Abort() error {
	if p.stage == "ABORTED" {
		return nil
	}
	if p.stage == "FINALIZE" || p.stage == "TIMED_OUT" {
		return errors.New("cannot abort a terminal DKG")
	}
	p.stage = "ABORTED"
	return nil
}

func (p *KyberParticipant) PublicResult() (KyberPublicResult, error) {
	if p.result == nil {
		return KyberPublicResult{}, errors.New("DKG not finalized")
	}
	group, err := p.result.Key.Public().MarshalBinary()
	if err != nil {
		return KyberPublicResult{}, err
	}
	publicSharePoint := p.suite.Point().Mul(p.result.Key.Share.V, nil)
	commitments := share.NewPubPoly(p.suite, p.suite.Point().Base(), p.result.Key.Commitments())
	if !commitments.Eval(int(p.index)).V.Equal(publicSharePoint) {
		return KyberPublicResult{}, errors.New("DKG private share does not match public commitments")
	}
	publicShare, err := publicSharePoint.MarshalBinary()
	if err != nil {
		return KyberPublicResult{}, err
	}
	return KyberPublicResult{ParticipantID: p.id, GroupPublic: group,
		PublicShare: publicShare, Qualified: len(p.result.QUAL)}, nil
}
