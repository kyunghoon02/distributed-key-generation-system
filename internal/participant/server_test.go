package participant_test

import (
	"context"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/api"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/cryptoadapter"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/participant"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/protocol"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/transport"
)

func TestDeliverySemanticsOverTCP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() { _ = participant.NewServer("p1", cryptoadapter.Mock{}).Serve(listener) }()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client := transport.TCP{Timeout: time.Second}
	address := listener.Addr().String()
	config := protocol.Config{SessionID: "tcp-session", Epoch: 1, Round: 1, Threshold: 2, Participants: []string{"p1", "p2"}}
	if _, err := client.Call(ctx, address, api.Request{Operation: "begin", Config: config}); err != nil {
		t.Fatalf("begin: %v", err)
	}
	sender := protocol.NewMachine("p2", cryptoadapter.Mock{})
	outbound, err := sender.Begin(config)
	if err != nil || len(outbound) != 1 {
		t.Fatalf("sender begin: outbound=%v err=%v", outbound, err)
	}
	message := outbound[0]
	if err := client.Send(ctx, address, message); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	before, err := client.Call(ctx, address, api.Request{Operation: "status"})
	if err != nil {
		t.Fatal(err)
	}
	if before.Status.Phase != protocol.PhaseVerify || before.Status.Received != 2 {
		t.Fatalf("status after delivery = %+v", before.Status)
	}
	if err := client.Send(ctx, address, message); err != nil {
		t.Fatalf("duplicate delivery: %v", err)
	}
	stale := message
	stale.Round = 0
	stale.MessageID = stale.LogicalID()
	if err := client.Send(ctx, address, stale); err == nil {
		t.Fatal("stale delivery was accepted")
	}
	after, err := client.Call(ctx, address, api.Request{Operation: "status"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after.Status, before.Status) {
		t.Fatalf("duplicate or stale delivery changed status: before=%+v after=%+v", before.Status, after.Status)
	}
}
