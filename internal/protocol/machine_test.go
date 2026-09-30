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
