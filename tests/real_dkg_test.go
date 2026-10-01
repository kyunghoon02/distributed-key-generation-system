package tests

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/realexperiment"
)

func TestRealDKGFourProcessFaultScenarios(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "dkgctl")
	build := exec.Command("go", "build", "-o", binary, "./cmd/dkgctl")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	for _, scenario := range []string{"E0", "E1", "E2", "E3", "E4", "E5", "E6"} {
		t.Run(scenario, func(t *testing.T) {
			command := exec.Command(binary, "real-run", "--scenario", scenario)
			command.Dir = root
			output, err := command.Output()
			if err != nil {
				if exit, ok := err.(*exec.ExitError); ok {
					t.Fatalf("real-run: %v\n%s", err, exit.Stderr)
				}
				t.Fatal(err)
			}
			var result realexperiment.Result
			if err := json.Unmarshal(output, &result); err != nil {
				t.Fatalf("decode: %v\n%s", err, output)
			}
			if len(result.ParticipantResults) != 4 || result.GroupKeyAgreement == "divergent" {
				t.Fatalf("invalid result: %+v", result)
			}
			switch scenario {
			case "E0":
				if result.FinalizedCount != 4 || result.GroupKeyAgreement != "consistent" {
					t.Fatalf("normal: %+v", result)
				}
			case "E1":
				if result.FinalizedCount != 4 || result.DuplicateCount != 1 {
					t.Fatalf("duplicate: %+v", result)
				}
			case "E2":
				if result.FinalizedCount != 4 || result.StaleRejectedCount != 1 {
					t.Fatalf("stale: %+v", result)
				}
			case "E3":
				if result.FinalizedCount != 4 || result.RecoveryMode != "fresh_session_after_abort" || result.AbortedSessions != 1 || result.RestartedProcesses != 4 || result.AbortedNonceSHA256 == result.SessionNonceSHA256 || result.RecoveryDurationMS == nil {
					t.Fatalf("fresh-session recovery: %+v", result)
				}
			case "E4":
				if result.FinalizedCount != 3 || result.ParticipantResults[3].Phase != "UNREACHABLE" {
					t.Fatalf("unavailable: %+v", result)
				}
			case "E5":
				if result.FinalizedCount != 0 || result.GroupKeyAgreement != "not_observed" || result.TerminalResult != "aborted" || result.AbortCount != 4 {
					t.Fatalf("partition: %+v", result)
				}
			case "E6":
				if result.FinalizedCount != 3 || result.LateRejectedCount != 3 || result.DelayedCount != 3 || result.HoldDurationMS < 100 {
					t.Fatalf("late: %+v", result)
				}
			}
		})
	}
}
