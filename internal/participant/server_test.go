package participant_test

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"net/http/httptest"
	"reflect"
	"strings"
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
	server := participant.NewServer("p1", cryptoadapter.Mock{})
	var logs bytes.Buffer
	server.SetLogger(slog.New(slog.NewJSONHandler(&logs, nil)))
	go func() { _ = server.Serve(listener) }()

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
	metrics := httptest.NewRecorder()
	server.MetricsHandler().ServeHTTP(metrics, httptest.NewRequest("GET", "/metrics", nil))
	if metrics.Code != 200 {
		t.Fatalf("metrics status = %d", metrics.Code)
	}
	for _, sample := range []string{
		`dkg_share_deliveries_total{outcome="applied"} 1`,
		`dkg_share_deliveries_total{outcome="duplicate"} 1`,
		`dkg_share_deliveries_total{outcome="stale"} 1`,
		`dkg_participant_phase_code 3`,
	} {
		if !strings.Contains(metrics.Body.String(), sample) {
			t.Fatalf("metrics missing %q:\n%s", sample, metrics.Body.String())
		}
	}
	if strings.Contains(metrics.Body.String(), config.SessionID) || strings.Contains(metrics.Body.String(), message.Payload) {
		t.Fatal("metrics exposed session or payload")
	}
	if !strings.Contains(logs.String(), `"operation":"deliver"`) ||
		!strings.Contains(logs.String(), `"session_id":"tcp-session"`) ||
		strings.Contains(logs.String(), message.Payload) {
		t.Fatalf("structured logs missing fields or exposed payload: %s", logs.String())
	}
}
