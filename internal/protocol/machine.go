package protocol

import (
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/cryptoadapter"
)

var (
	ErrInvalidConfig  = errors.New("invalid ceremony config")
	ErrInvalidPhase   = errors.New("invalid protocol phase")
	ErrInvalidMessage = errors.New("invalid protocol message")
	ErrStaleMessage   = errors.New("stale protocol message")
	ErrNotReady       = errors.New("participant is not ready to finalize")
)

type Status struct {
	ParticipantID string  `json:"participant_id"`
	SessionID     string  `json:"session_id,omitempty"`
	Phase         Phase   `json:"phase"`
	Received      int     `json:"received"`
	Expected      int     `json:"expected"`
	Threshold     int     `json:"threshold"`
	Transitions   []Phase `json:"transitions"`
}

type Machine struct {
	mu            sync.Mutex
	id            string
	crypto        cryptoadapter.Adapter
	config        Config
	phase         Phase
	journal       Journal
	contributions map[string]string
	applied       map[string]string
	transitions   []Phase
}

func NewMachine(participantID string, adapter cryptoadapter.Adapter) *Machine {
	return &Machine{
		id:            participantID,
		crypto:        adapter,
		phase:         PhaseInit,
		contributions: make(map[string]string),
		applied:       make(map[string]string),
		transitions:   []Phase{PhaseInit},
	}
}

func (m *Machine) Begin(config Config) ([]Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.phase != PhaseInit {
		if sameConfig(config, m.config) {
			return m.outbound(), nil
		}
		return nil, fmt.Errorf("begin from %s: %w", m.phase, ErrInvalidPhase)
	}
	if err := validateConfig(config, m.id); err != nil {
		return nil, err
	}
	contribution, err := m.crypto.CreateContribution(config.SessionID, m.id)
	if err != nil {
		return nil, fmt.Errorf("create mock contribution: %w", err)
	}
	if m.journal != nil {
		if err := m.journal.Append(Event{Type: EventBegin, ParticipantID: m.id, Config: config, Contribution: contribution}); err != nil {
			return nil, fmt.Errorf("record begin: %w", err)
		}
	}
	return m.applyBegin(config, contribution)
}

func (m *Machine) applyBegin(config Config, contribution string) ([]Message, error) {
	if m.phase != PhaseInit {
		return nil, ErrInvalidPhase
	}
	if err := validateConfig(config, m.id); err != nil {
		return nil, err
	}
	m.config = config
	m.config.Participants = slices.Clone(config.Participants)
	m.contributions[m.id] = contribution
	if err := m.transition(PhaseDeal); err != nil {
		return nil, err
	}
	if err := m.transition(PhaseShareExchange); err != nil {
		return nil, err
	}

	if len(m.contributions) == len(config.Participants) {
		if err := m.transition(PhaseVerify); err != nil {
			return nil, err
		}
	}
	return m.outbound(), nil
}

func (m *Machine) outbound() []Message {
	outbound := make([]Message, 0, len(m.config.Participants)-1)
	for _, peerID := range m.config.Participants {
		if peerID == m.id {
			continue
		}
		message := Message{
			SessionID: m.config.SessionID,
			Epoch:     m.config.Epoch,
			Round:     m.config.Round,
			Phase:     PhaseShareExchange,
			From:      m.id,
			To:        peerID,
			Payload:   m.contributions[m.id],
		}
		message.MessageID = message.LogicalID()
		outbound = append(outbound, message)
	}
	return outbound
}

func (m *Machine) ReceiveShare(message Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.phase == PhaseInit {
		return fmt.Errorf("receive share in %s: %w", m.phase, ErrInvalidPhase)
	}
	if message.SessionID != m.config.SessionID || message.Epoch != m.config.Epoch || message.Round != m.config.Round {
		return ErrStaleMessage
	}
	if message.Phase != PhaseShareExchange || message.To != m.id || message.From == m.id ||
		!slices.Contains(m.config.Participants, message.From) || message.MessageID == "" ||
		message.MessageID != message.LogicalID() {
		return ErrInvalidMessage
	}
	if appliedPayload, ok := m.applied[message.MessageID]; ok {
		if appliedPayload != message.Payload {
			return ErrInvalidMessage
		}
		return nil
	}
	if m.phase != PhaseShareExchange {
		return fmt.Errorf("receive share in %s: %w", m.phase, ErrStaleMessage)
	}
	if !m.crypto.VerifyContribution(m.config.SessionID, message.From, message.Payload) {
		return ErrInvalidMessage
	}
	if m.journal != nil {
		if err := m.journal.Append(Event{Type: EventShare, Message: message}); err != nil {
			return fmt.Errorf("record share: %w", err)
		}
	}
	m.applied[message.MessageID] = message.Payload
	m.contributions[message.From] = message.Payload
	if len(m.contributions) == len(m.config.Participants) {
		return m.transition(PhaseVerify)
	}
	return nil
}

func (m *Machine) Finalize() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.phase != PhaseVerify {
		return fmt.Errorf("finalize from %s: %w", m.phase, ErrNotReady)
	}
	if len(m.contributions) < m.config.Threshold {
		return fmt.Errorf("have %d contributions, need %d: %w", len(m.contributions), m.config.Threshold, ErrNotReady)
	}
	for _, participantID := range m.config.Participants {
		contribution, ok := m.contributions[participantID]
		if !ok || !m.crypto.VerifyContribution(m.config.SessionID, participantID, contribution) {
			return fmt.Errorf("invalid contribution from %q: %w", participantID, ErrInvalidMessage)
		}
	}
	if m.journal != nil {
		if err := m.journal.Append(Event{Type: EventFinalize}); err != nil {
			return fmt.Errorf("record finalize: %w", err)
		}
	}
	return m.transition(PhaseFinalize)
}

func sameConfig(a, b Config) bool {
	return a.SessionID == b.SessionID && a.Epoch == b.Epoch && a.Round == b.Round &&
		a.Threshold == b.Threshold && slices.Equal(a.Participants, b.Participants)
}

func (m *Machine) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	return Status{
		ParticipantID: m.id,
		SessionID:     m.config.SessionID,
		Phase:         m.phase,
		Received:      len(m.contributions),
		Expected:      len(m.config.Participants),
		Threshold:     m.config.Threshold,
		Transitions:   append([]Phase(nil), m.transitions...),
	}
}

func (m *Machine) transition(next Phase) error {
	allowed := map[Phase]Phase{
		PhaseInit:          PhaseDeal,
		PhaseDeal:          PhaseShareExchange,
		PhaseShareExchange: PhaseVerify,
		PhaseVerify:        PhaseFinalize,
	}
	if allowed[m.phase] != next {
		return fmt.Errorf("%s -> %s: %w", m.phase, next, ErrInvalidPhase)
	}
	m.phase = next
	m.transitions = append(m.transitions, next)
	return nil
}

func validateConfig(config Config, participantID string) error {
	if config.SessionID == "" || len(config.Participants) == 0 || config.Threshold < 1 || config.Threshold > len(config.Participants) {
		return ErrInvalidConfig
	}
	if !slices.Contains(config.Participants, participantID) {
		return fmt.Errorf("participant %q is not a member: %w", participantID, ErrInvalidConfig)
	}
	seen := make(map[string]struct{}, len(config.Participants))
	for _, id := range config.Participants {
		if id == "" {
			return ErrInvalidConfig
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("duplicate participant %q: %w", id, ErrInvalidConfig)
		}
		seen[id] = struct{}{}
	}
	return nil
}
