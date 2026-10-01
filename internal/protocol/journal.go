package protocol

import (
	"fmt"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/cryptoadapter"
)

type EventType string

const (
	EventBegin    EventType = "begin"
	EventShare    EventType = "share"
	EventFinalize EventType = "finalize"
)

// Event records an accepted state change. Begin includes the participant's
// generated contribution so replay never generates it again.
type Event struct {
	Type          EventType `json:"type"`
	ParticipantID string    `json:"participant_id,omitempty"`
	Config        Config    `json:"config,omitempty"`
	Contribution  string    `json:"contribution,omitempty"`
	Message       Message   `json:"message,omitempty"`
}

// Journal must make an appended event durable before Append returns nil.
type Journal interface {
	Append(Event) error
	ReadAll() ([]Event, error)
}

func RecoverMachine(participantID string, adapter cryptoadapter.Adapter, journal Journal) (*Machine, error) {
	events, err := journal.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read journal: %w", err)
	}
	machine := NewMachine(participantID, adapter)
	for index, event := range events {
		switch event.Type {
		case EventBegin:
			if event.ParticipantID != participantID || event.Contribution == "" ||
				!adapter.VerifyContribution(event.Config.SessionID, participantID, event.Contribution) {
				err = ErrInvalidMessage
			} else {
				_, err = machine.applyBegin(event.Config, event.Contribution)
			}
		case EventShare:
			err = machine.ReceiveShare(event.Message)
		case EventFinalize:
			err = machine.Finalize()
		default:
			err = fmt.Errorf("unknown event type %q", event.Type)
		}
		if err != nil {
			return nil, fmt.Errorf("replay event %d (%s): %w", index+1, event.Type, err)
		}
	}
	machine.journal = journal
	return machine, nil
}
