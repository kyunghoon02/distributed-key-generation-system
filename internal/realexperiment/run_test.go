package realexperiment

import "testing"

func TestRealDKGFaultSchedules(t *testing.T) {
	for _, scenario := range []string{"E0", "E1", "E2", "E4", "E5", "E6"} {
		t.Run(scenario, func(t *testing.T) {
			result, err := Run(scenario)
			if err != nil {
				t.Fatal(err)
			}
			if result.GroupKeyAgreement == "divergent" {
				t.Fatalf("group public keys disagree: %+v", result)
			}
			if scenario == "E0" && result.FinalizedCount != 4 {
				t.Fatalf("normal run: %+v", result)
			}
			if scenario == "E1" && (result.FinalizedCount != 4 || result.DuplicateCount != 1) {
				t.Fatalf("duplicate: %+v", result)
			}
			if scenario == "E2" && (result.FinalizedCount != 4 || result.StaleRejectedCount != 1) {
				t.Fatalf("stale: %+v", result)
			}
			if scenario == "E5" && result.FinalizedCount != 0 {
				t.Fatalf("partition finalized: %+v", result)
			}
			if scenario == "E5" && (result.TerminalResult != "aborted" || result.AbortCount != 4) {
				t.Fatalf("partition did not abort: %+v", result)
			}
			if scenario == "E6" && (result.LateRejectedCount != 3 || result.DelayedCount != 3 || result.HoldDurationMS < 100) {
				t.Fatalf("late delivery: %+v", result)
			}
		})
	}
}
