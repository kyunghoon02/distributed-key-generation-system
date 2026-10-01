package tests

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/protocol"
)

type runResult struct {
	SessionID    string            `json:"session_id"`
	Participants []protocol.Status `json:"participants"`
	Finalized    bool              `json:"finalized"`
}

func TestNormalCeremonyFourParticipantProcesses(t *testing.T) {
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
	run := exec.CommandContext(ctx, binary, "run", "--participants", "4", "--threshold", "3")
	run.Dir = root
	output, err := run.Output()
	if err != nil {
		if ctx.Err() != nil {
			t.Fatalf("run dkgctl: %v (timed out)\n%s", err, output)
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("run dkgctl: %v\nstderr:\n%s\nstdout:\n%s", err, exitErr.Stderr, output)
		}
		t.Fatalf("run dkgctl: %v", err)
	}

	var result runResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode run result: %v\n%s", err, output)
	}
	if !result.Finalized {
		t.Fatal("ceremony did not finalize")
	}
	if len(result.Participants) != 4 {
		t.Fatalf("participants = %d, want 4", len(result.Participants))
	}
	wantTransitions := []protocol.Phase{
		protocol.PhaseInit,
		protocol.PhaseDeal,
		protocol.PhaseShareExchange,
		protocol.PhaseVerify,
		protocol.PhaseFinalize,
	}
	for _, status := range result.Participants {
		if status.Phase != protocol.PhaseFinalize {
			t.Errorf("%s ended in %s", status.ParticipantID, status.Phase)
		}
		if status.Received != 4 || status.Expected != 4 || status.Threshold != 3 {
			t.Errorf("%s status = received %d / %d, threshold %d; want 4 / 4, threshold 3", status.ParticipantID, status.Received, status.Expected, status.Threshold)
		}
		if !reflect.DeepEqual(status.Transitions, wantTransitions) {
			t.Errorf("%s transitions = %v, want %v", status.ParticipantID, status.Transitions, wantTransitions)
		}
	}
}
