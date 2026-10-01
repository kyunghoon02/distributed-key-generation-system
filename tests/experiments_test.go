package tests

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/protocol"
)

type experimentOutput struct {
	ExperimentID       string            `json:"experiment_id"`
	Revision           string            `json:"revision"`
	TerminalResult     string            `json:"terminal_result"`
	PhaseDurationMS    map[string]int64  `json:"phase_duration_ms"`
	DuplicateCount     int               `json:"duplicate_count"`
	StaleMessageCount  int               `json:"stale_message_count"`
	TimeoutCount       int               `json:"timeout_count"`
	RecoveryDurationMS *int64            `json:"recovery_duration_ms"`
	Participants       []protocol.Status `json:"participants"`
}

func TestDeterministicExperimentScenarios(t *testing.T) {
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
	for _, scenario := range []string{"E0", "E1", "E2", "E3", "E4", "E5", "E6"} {
		t.Run(scenario, func(t *testing.T) {
			command := exec.Command(binary, "experiment", "--scenario", scenario, "--phase-deadline", "100ms")
			command.Dir = root
			output, err := command.Output()
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					t.Fatalf("experiment %s: %v\nstderr:\n%s", scenario, err, exitErr.Stderr)
				}
				t.Fatalf("experiment %s: %v", scenario, err)
			}
			var result experimentOutput
			if err := json.Unmarshal(output, &result); err != nil {
				t.Fatalf("decode %s: %v\n%s", scenario, err, output)
			}
			if result.ExperimentID != scenario {
				t.Fatalf("experiment ID = %q, want %q", result.ExperimentID, scenario)
			}
			if result.Revision == "" || len(result.PhaseDurationMS) == 0 {
				t.Fatalf("missing revision or phase duration: %+v", result)
			}
			if scenario <= "E3" {
				if result.TerminalResult != "completed" || len(result.Participants) != 4 {
					t.Fatalf("result = %+v, want four completed participants", result)
				}
				for _, status := range result.Participants {
					if status.Phase != protocol.PhaseFinalize {
						t.Fatalf("%s phase = %s, want FINALIZE", status.ParticipantID, status.Phase)
					}
				}
			} else {
				if result.TerminalResult != "timed_out" || result.TimeoutCount == 0 {
					t.Fatalf("result = %+v, want terminal timeout", result)
				}
				for _, status := range result.Participants {
					if status.Phase != protocol.PhaseTimedOut {
						t.Fatalf("%s phase = %s, want TIMED_OUT", status.ParticipantID, status.Phase)
					}
				}
			}
			switch scenario {
			case "E0":
				path := filepath.Join(t.TempDir(), "E0.json")
				textRun := exec.Command(binary, "experiment", "--scenario", "E0", "--format", "text", "--output", path)
				textRun.Dir = root
				textOutput, err := textRun.Output()
				if err != nil || !strings.Contains(string(textOutput), "E0: completed") {
					t.Fatalf("text output = %q, err=%v", textOutput, err)
				}
				file, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var saved experimentOutput
				if err := json.Unmarshal(file, &saved); err != nil || saved.ExperimentID != "E0" {
					t.Fatalf("saved result = %+v, err=%v", saved, err)
				}
			case "E1":
				if result.DuplicateCount != 1 {
					t.Fatalf("duplicate count = %d, want 1", result.DuplicateCount)
				}
			case "E2":
				if result.StaleMessageCount != 1 {
					t.Fatalf("stale count = %d, want 1", result.StaleMessageCount)
				}
			case "E3":
				if result.RecoveryDurationMS == nil {
					t.Fatal("recovery duration missing")
				}
			case "E5":
				if len(result.Participants) != 4 {
					t.Fatalf("participants = %d, want 4", len(result.Participants))
				}
				for _, status := range result.Participants {
					if status.Received != 2 {
						t.Fatalf("%s received %d shares, want 2 within partition", status.ParticipantID, status.Received)
					}
				}
			}
		})
	}
}
