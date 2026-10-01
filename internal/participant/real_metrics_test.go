package participant

import (
	"encoding/json"
	"net"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/api"
)

func TestRealMetricsUseBoundedLabels(t *testing.T) {
	server, err := NewRealServer("p1", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"real-status", "arbitrary-user-operation"} {
		client, peer := net.Pipe()
		go server.handle(peer)
		if err := json.NewEncoder(client).Encode(api.Request{Operation: operation}); err != nil {
			t.Fatal(err)
		}
		var response api.Response
		if err := json.NewDecoder(client).Decode(&response); err != nil {
			t.Fatal(err)
		}
		client.Close()
	}
	metrics := httptest.NewRecorder()
	server.MetricsHandler().ServeHTTP(metrics, httptest.NewRequest("GET", "/metrics", nil))
	if metrics.Code != 200 {
		t.Fatalf("metrics status: %d", metrics.Code)
	}
	for _, expected := range []string{
		`real_dkg_requests_total{operation="real-status",result="success"} 1`,
		`real_dkg_requests_total{operation="unknown",result="error"} 1`,
		`real_dkg_participant_phase_code 0`,
	} {
		if !strings.Contains(metrics.Body.String(), expected) {
			t.Fatalf("missing %q in metrics: %s", expected, metrics.Body.String())
		}
	}
	if strings.Contains(metrics.Body.String(), "arbitrary-user-operation") {
		t.Fatal("unbounded operation reached metrics label")
	}
}
