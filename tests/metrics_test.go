package tests

import (
	"context"
	"io"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/api"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/protocol"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/transport"
)

func TestParticipantMetricsEndpoint(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "dkgctl")
	build := exec.Command("go", "build", "-o", binary, "./cmd/dkgctl")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build dkgctl: %v\n%s", err, output)
	}
	rpcAddress := testAddress(t)
	metricsAddress := testAddress(t)
	command := exec.Command(binary, "participant", "--id", "p1", "--listen", rpcAddress, "--metrics-listen", metricsAddress)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	client := transport.TCP{Timeout: time.Second}
	waitForRecoveryProcess(t, ctx, client, rpcAddress)
	config := protocol.Config{SessionID: "metrics-session", Epoch: 1, Round: 1, Threshold: 2, Participants: []string{"p1", "p2"}}
	if _, err := client.Call(ctx, rpcAddress, api.Request{Operation: "begin", Config: config}); err != nil {
		t.Fatal(err)
	}
	httpClient := http.Client{Timeout: time.Second}
	var body string
	for {
		response, err := httpClient.Get("http://" + metricsAddress + "/metrics")
		if err == nil {
			data, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if readErr == nil && response.StatusCode == http.StatusOK {
				body = string(data)
				break
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("metrics endpoint unavailable: %v", ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
	if !strings.Contains(body, "dkg_participant_phase_code 2") ||
		!strings.Contains(body, `dkg_requests_total{operation="begin",result="success"} 1`) ||
		strings.Contains(body, config.SessionID) {
		t.Fatalf("unexpected metrics output:\n%s", body)
	}
}

func testAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}
