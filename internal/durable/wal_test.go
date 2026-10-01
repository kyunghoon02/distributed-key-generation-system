package durable

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/protocol"
)

func TestWALDiscardsIncompleteTailAndKeepsCompleteEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "participant.wal")
	wal, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	first := protocol.Event{Type: protocol.EventBegin, ParticipantID: "p1"}
	if err := wal.Append(first); err != nil {
		t.Fatal(err)
	}
	if err := wal.Close(); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"type":"share","message":`); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	wal, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wal.Close()
	events, err := wal.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != first.Type {
		t.Fatalf("replayed events = %+v, want only the complete begin event", events)
	}
	if err := wal.Append(protocol.Event{Type: protocol.EventFinalize}); err != nil {
		t.Fatal(err)
	}
	events, err = wal.ReadAll()
	if err != nil || len(events) != 2 || events[1].Type != protocol.EventFinalize {
		t.Fatalf("events after repair and append = %+v, err=%v", events, err)
	}
}

func TestWALRejectsMalformedCompleteRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "participant.wal")
	if err := os.WriteFile(path, []byte("{invalid}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	wal, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer wal.Close()
	if _, err := wal.ReadAll(); err == nil {
		t.Fatal("malformed complete record was accepted")
	}
}
