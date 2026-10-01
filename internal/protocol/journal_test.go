package protocol

import (
	"errors"
	"reflect"
	"testing"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/cryptoadapter"
)

type memoryJournal struct {
	events      []Event
	appendError error
}

func (j *memoryJournal) Append(event Event) error {
	if j.appendError != nil {
		return j.appendError
	}
	j.events = append(j.events, event)
	return nil
}

func (j *memoryJournal) ReadAll() ([]Event, error) {
	return append([]Event(nil), j.events...), nil
}

func TestReplayRestoresStateAndMessageIdentity(t *testing.T) {
	config := Config{SessionID: "replay-session", Epoch: 1, Round: 1, Threshold: 2, Participants: []string{"p1", "p2", "p3"}}
	journal := &memoryJournal{}
	machine, err := RecoverMachine("p1", cryptoadapter.Mock{}, journal)
	if err != nil {
		t.Fatal(err)
	}
	outbound, err := machine.Begin(config)
	if err != nil {
		t.Fatal(err)
	}
	first := outboundShare(t, config, "p2", "p1")
	if err := machine.ReceiveShare(first); err != nil {
		t.Fatal(err)
	}
	before := machine.Status()
	if len(journal.events) != 2 {
		t.Fatalf("events = %d, want begin and share", len(journal.events))
	}

	recovered, err := RecoverMachine("p1", cryptoadapter.Mock{}, journal)
	if err != nil {
		t.Fatal(err)
	}
	if got := recovered.Status(); !reflect.DeepEqual(got, before) {
		t.Fatalf("replay changed status: before=%+v after=%+v", before, got)
	}
	replayedOutbound, err := recovered.Begin(config)
	if err != nil || !reflect.DeepEqual(replayedOutbound, outbound) {
		t.Fatalf("retry begin: outbound=%+v, err=%v", replayedOutbound, err)
	}
	if err := recovered.ReceiveShare(first); err != nil {
		t.Fatalf("duplicate after replay: %v", err)
	}
	if len(journal.events) != 2 || !reflect.DeepEqual(recovered.Status(), before) {
		t.Fatal("retry begin or duplicate changed recovered state")
	}
	if err := recovered.ReceiveShare(outboundShare(t, config, "p3", "p1")); err != nil {
		t.Fatal(err)
	}
	if err := recovered.Finalize(); err != nil {
		t.Fatal(err)
	}
	finalStatus := recovered.Status()
	terminal, err := RecoverMachine("p1", cryptoadapter.Mock{}, journal)
	if err != nil {
		t.Fatalf("terminal replay: %v", err)
	}
	if got := terminal.Status(); !reflect.DeepEqual(got, finalStatus) {
		t.Fatalf("terminal replay status=%+v, want %+v", got, finalStatus)
	}
}

func TestJournalFailureDoesNotMutateState(t *testing.T) {
	config := Config{SessionID: "write-failure", Epoch: 1, Round: 1, Threshold: 2, Participants: []string{"p1", "p2"}}
	journal := &memoryJournal{}
	machine, err := RecoverMachine("p1", cryptoadapter.Mock{}, journal)
	if err != nil {
		t.Fatal(err)
	}
	journal.appendError = errors.New("disk unavailable")
	if _, err := machine.Begin(config); err == nil || machine.Status().Phase != PhaseInit {
		t.Fatalf("failed begin changed state: err=%v status=%+v", err, machine.Status())
	}
	journal.appendError = nil
	if _, err := machine.Begin(config); err != nil {
		t.Fatal(err)
	}
	before := machine.Status()
	journal.appendError = errors.New("disk unavailable")
	if err := machine.ReceiveShare(outboundShare(t, config, "p2", "p1")); err == nil {
		t.Fatal("share write failure was accepted")
	}
	if got := machine.Status(); !reflect.DeepEqual(got, before) {
		t.Fatalf("failed share changed state: before=%+v after=%+v", before, got)
	}
	journal.appendError = nil
	if err := machine.ReceiveShare(outboundShare(t, config, "p2", "p1")); err != nil {
		t.Fatal(err)
	}
	before = machine.Status()
	journal.appendError = errors.New("disk unavailable")
	if err := machine.Finalize(); err == nil {
		t.Fatal("finalize write failure was accepted")
	}
	if got := machine.Status(); !reflect.DeepEqual(got, before) {
		t.Fatalf("failed finalize changed state: before=%+v after=%+v", before, got)
	}
}
