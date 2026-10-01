package protocol

import (
	"errors"
	"reflect"
	"testing"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/cryptoadapter"
)

func TestMachineRequiresAllSharesBeforeVerify(t *testing.T) {
	machine := NewMachine("p1", cryptoadapter.Mock{})
	config := Config{
		SessionID:    "test-session",
		Epoch:        1,
		Round:        1,
		Threshold:    1,
		Participants: []string{"p1", "p2"},
	}
	if _, err := machine.Begin(config); err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := machine.Finalize(); !errors.Is(err, ErrNotReady) {
		t.Fatalf("Finalize error = %v, want ErrNotReady", err)
	}

	status := machine.Status()
	wantTransitions := []Phase{PhaseInit, PhaseDeal, PhaseShareExchange}
	if status.Phase != PhaseShareExchange {
		t.Fatalf("phase = %s, want %s", status.Phase, PhaseShareExchange)
	}
	if !reflect.DeepEqual(status.Transitions, wantTransitions) {
		t.Fatalf("transitions = %v, want %v", status.Transitions, wantTransitions)
	}
}

func TestSingleParticipantCeremonyCanFinalize(t *testing.T) {
	machine := NewMachine("p1", cryptoadapter.Mock{})
	config := Config{
		SessionID:    "single-session",
		Epoch:        1,
		Round:        1,
		Threshold:    1,
		Participants: []string{"p1"},
	}
	if _, err := machine.Begin(config); err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := machine.Finalize(); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if got := machine.Status().Phase; got != PhaseFinalize {
		t.Fatalf("phase = %s, want %s", got, PhaseFinalize)
	}
}

func TestDuplicateShareAppliesOnceAcrossPhases(t *testing.T) {
	config := Config{SessionID: "duplicate-session", Epoch: 1, Round: 1, Threshold: 2, Participants: []string{"p1", "p2", "p3"}}
	receiver := NewMachine("p1", cryptoadapter.Mock{})
	if _, err := receiver.Begin(config); err != nil {
		t.Fatal(err)
	}
	first := outboundShare(t, config, "p2", "p1")
	second := outboundShare(t, config, "p3", "p1")
	if first.MessageID == "" || first.MessageID != first.LogicalID() {
		t.Fatalf("outbound message has invalid ID: %q", first.MessageID)
	}
	if err := receiver.ReceiveShare(first); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	assertDuplicateIsNoOp(t, receiver, first)
	if err := receiver.ReceiveShare(second); err != nil {
		t.Fatalf("second sender: %v", err)
	}
	if got := receiver.Status().Phase; got != PhaseVerify {
		t.Fatalf("phase = %s, want VERIFY", got)
	}
	assertDuplicateIsNoOp(t, receiver, first)
	if err := receiver.Finalize(); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	assertDuplicateIsNoOp(t, receiver, first)
	if got := receiver.Status().Transitions; !reflect.DeepEqual(got, []Phase{PhaseInit, PhaseDeal, PhaseShareExchange, PhaseVerify, PhaseFinalize}) {
		t.Fatalf("transitions = %v", got)
	}
}

func TestRejectedSharesDoNotMutateState(t *testing.T) {
	config := Config{SessionID: "validation-session", Epoch: 2, Round: 3, Threshold: 2, Participants: []string{"p1", "p2"}}
	valid := outboundShare(t, config, "p2", "p1")
	tests := []struct {
		name string
		edit func(*Message)
		want error
	}{
		{"session", func(m *Message) { m.SessionID = "previous"; m.MessageID = m.LogicalID() }, ErrStaleMessage},
		{"epoch", func(m *Message) { m.Epoch--; m.MessageID = m.LogicalID() }, ErrStaleMessage},
		{"round", func(m *Message) { m.Round--; m.MessageID = m.LogicalID() }, ErrStaleMessage},
		{"phase", func(m *Message) { m.Phase = PhaseDeal; m.MessageID = m.LogicalID() }, ErrInvalidMessage},
		{"sender", func(m *Message) { m.From = "outsider"; m.MessageID = m.LogicalID() }, ErrInvalidMessage},
		{"recipient", func(m *Message) { m.To = "outsider"; m.MessageID = m.LogicalID() }, ErrInvalidMessage},
		{"missing ID", func(m *Message) { m.MessageID = "" }, ErrInvalidMessage},
		{"forged ID", func(m *Message) { m.MessageID = "wrong" }, ErrInvalidMessage},
		{"bad contribution", func(m *Message) { m.Payload = "bad" }, ErrInvalidMessage},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			receiver := NewMachine("p1", cryptoadapter.Mock{})
			if _, err := receiver.Begin(config); err != nil {
				t.Fatal(err)
			}
			before := receiver.Status()
			message := valid
			test.edit(&message)
			if err := receiver.ReceiveShare(message); !errors.Is(err, test.want) {
				t.Fatalf("receive error = %v, want %v", err, test.want)
			}
			if got := receiver.Status(); !reflect.DeepEqual(got, before) {
				t.Fatalf("rejected message changed status: before=%+v after=%+v", before, got)
			}
			if err := receiver.ReceiveShare(valid); err != nil {
				t.Fatalf("valid message after rejection: %v", err)
			}
		})
	}
}

func TestConflictingDuplicateIsRejected(t *testing.T) {
	config := Config{SessionID: "conflict-session", Epoch: 1, Round: 1, Threshold: 2, Participants: []string{"p1", "p2"}}
	receiver := NewMachine("p1", cryptoadapter.Mock{})
	if _, err := receiver.Begin(config); err != nil {
		t.Fatal(err)
	}
	message := outboundShare(t, config, "p2", "p1")
	if err := receiver.ReceiveShare(message); err != nil {
		t.Fatal(err)
	}
	before := receiver.Status()
	message.Payload = "conflicting payload"
	if err := receiver.ReceiveShare(message); !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("conflicting duplicate error = %v, want ErrInvalidMessage", err)
	}
	if got := receiver.Status(); !reflect.DeepEqual(got, before) {
		t.Fatalf("conflicting duplicate changed status: before=%+v after=%+v", before, got)
	}
}

func outboundShare(t *testing.T, config Config, from, to string) Message {
	t.Helper()
	sender := NewMachine(from, cryptoadapter.Mock{})
	outbound, err := sender.Begin(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range outbound {
		if message.To == to {
			return message
		}
	}
	t.Fatalf("no outbound share from %s to %s", from, to)
	return Message{}
}

func assertDuplicateIsNoOp(t *testing.T, receiver *Machine, message Message) {
	t.Helper()
	before := receiver.Status()
	if err := receiver.ReceiveShare(message); err != nil {
		t.Fatalf("duplicate delivery: %v", err)
	}
	if got := receiver.Status(); !reflect.DeepEqual(got, before) {
		t.Fatalf("duplicate changed status: before=%+v after=%+v", before, got)
	}
}
