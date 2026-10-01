package protocol

import "testing"

func TestLogicalMessageID(t *testing.T) {
	message := Message{
		SessionID: "session|with:separators",
		Epoch:     1,
		Round:     2,
		Phase:     PhaseShareExchange,
		From:      "p1",
		To:        "p2",
		Payload:   "first delivery",
	}
	id := message.LogicalID()
	if id == "" || id != message.LogicalID() {
		t.Fatalf("logical ID is empty or unstable: %q", id)
	}
	message.Payload = "conflicting delivery"
	if got := message.LogicalID(); got != id {
		t.Fatalf("payload changed logical ID: %q, want %q", got, id)
	}
	message.To = "p3"
	if got := message.LogicalID(); got == id {
		t.Fatal("different recipient reused logical ID")
	}
}
