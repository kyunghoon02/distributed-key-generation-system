package tests

import (
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/api"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/protocol"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/transport"
)

type recoveryProcess struct {
	address string
	command *exec.Cmd
	stopped bool
}

func (p *recoveryProcess) stop() {
	if p.stopped {
		return
	}
	p.stopped = true
	_ = p.command.Process.Kill()
	_ = p.command.Wait()
}

func TestCrashRecoveryDuringShare(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client := transport.TCP{Timeout: time.Second}
	ids := []string{"p1", "p2", "p3"}
	addresses := make(map[string]string, len(ids))
	processes := make(map[string]*recoveryProcess, len(ids))
	stateDir := t.TempDir()
	for _, id := range ids {
		processes[id] = startRecoveryProcess(t, ctx, binary, id, filepath.Join(stateDir, id+".wal"))
		addresses[id] = processes[id].address
		waitForRecoveryProcess(t, ctx, client, processes[id].address)
	}
	config := protocol.Config{SessionID: "m2-recovery", Epoch: 1, Round: 1, Threshold: 2, Participants: ids}
	var outboxes []protocol.Message
	var p1Outbound []protocol.Message
	for _, id := range ids {
		response, err := client.Call(ctx, addresses[id], api.Request{Operation: "begin", Config: config})
		if err != nil {
			t.Fatalf("begin %s: %v", id, err)
		}
		outboxes = append(outboxes, response.Outbound...)
		if id == "p1" {
			p1Outbound = response.Outbound
		}
	}
	var firstShare protocol.Message
	for _, message := range outboxes {
		if message.From == "p2" && message.To == "p1" {
			firstShare = message
			break
		}
	}
	if firstShare.MessageID == "" {
		t.Fatal("missing p2 to p1 share")
	}
	if err := client.Send(ctx, addresses["p1"], firstShare); err != nil {
		t.Fatalf("first share: %v", err)
	}
	before := recoveryStatus(t, ctx, client, addresses["p1"])
	if before.Phase != protocol.PhaseShareExchange || before.Received != 2 {
		t.Fatalf("state before crash = %+v", before)
	}
	processes["p1"].stop()
	stateFile := filepath.Join(stateDir, "p1.wal")
	journalBefore := fileSize(t, stateFile)

	restartAt := time.Now()
	processes["p1"] = startRecoveryProcess(t, ctx, binary, "p1", stateFile)
	addresses["p1"] = processes["p1"].address
	waitForRecoveryProcess(t, ctx, client, addresses["p1"])
	recovered := recoveryStatus(t, ctx, client, addresses["p1"])
	recoveryDuration := time.Since(restartAt)
	t.Logf("recovery_duration=%s (restart to first recovered status)", recoveryDuration)
	if !reflect.DeepEqual(recovered, before) {
		t.Fatalf("replayed state = %+v, want %+v", recovered, before)
	}
	retry, err := client.Call(ctx, addresses["p1"], api.Request{Operation: "begin", Config: config})
	if err != nil || !reflect.DeepEqual(retry.Outbound, p1Outbound) {
		t.Fatalf("retry begin after recovery: outbound=%+v err=%v", retry.Outbound, err)
	}
	if err := client.Send(ctx, addresses["p1"], firstShare); err != nil {
		t.Fatalf("duplicate share after recovery: %v", err)
	}
	if got := recoveryStatus(t, ctx, client, addresses["p1"]); !reflect.DeepEqual(got, before) {
		t.Fatalf("duplicate changed recovered state: %+v", got)
	}
	if got := fileSize(t, stateFile); got != journalBefore {
		t.Fatalf("retry or duplicate appended journal event: before=%d after=%d", journalBefore, got)
	}

	for _, message := range outboxes {
		if err := client.Send(ctx, addresses[message.To], message); err != nil {
			t.Fatalf("deliver %s to %s: %v", message.From, message.To, err)
		}
	}
	for _, id := range ids {
		if _, err := client.Call(ctx, addresses[id], api.Request{Operation: "finalize"}); err != nil {
			t.Fatalf("finalize %s: %v", id, err)
		}
		status := recoveryStatus(t, ctx, client, addresses[id])
		if status.Phase != protocol.PhaseFinalize || status.Received != len(ids) ||
			!reflect.DeepEqual(status.Transitions, []protocol.Phase{protocol.PhaseInit, protocol.PhaseDeal, protocol.PhaseShareExchange, protocol.PhaseVerify, protocol.PhaseFinalize}) {
			t.Fatalf("terminal state for %s = %+v", id, status)
		}
	}
	finalBefore := recoveryStatus(t, ctx, client, addresses["p1"])
	processes["p1"].stop()
	processes["p1"] = startRecoveryProcess(t, ctx, binary, "p1", stateFile)
	waitForRecoveryProcess(t, ctx, client, processes["p1"].address)
	if got := recoveryStatus(t, ctx, client, processes["p1"].address); !reflect.DeepEqual(got, finalBefore) {
		t.Fatalf("terminal replay = %+v, want %+v", got, finalBefore)
	}
}

func startRecoveryProcess(t *testing.T, ctx context.Context, binary, id, stateFile string) *recoveryProcess {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, binary, "participant", "--id", id, "--listen", address, "--state-file", stateFile)
	command.Stdout = io.Discard
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start %s: %v", id, err)
	}
	process := &recoveryProcess{address: address, command: command}
	t.Cleanup(process.stop)
	return process
}

func waitForRecoveryProcess(t *testing.T, parent context.Context, client transport.TCP, address string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, 4*time.Second)
	defer cancel()
	for {
		if _, err := client.Call(ctx, address, api.Request{Operation: "status"}); err == nil {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("participant at %s did not become ready: %v", address, ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func recoveryStatus(t *testing.T, ctx context.Context, client transport.TCP, address string) protocol.Status {
	t.Helper()
	response, err := client.Call(ctx, address, api.Request{Operation: "status"})
	if err != nil {
		t.Fatalf("status at %s: %v", address, err)
	}
	return response.Status
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}
