package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"time"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/api"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/protocol"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/transport"
)

type experimentResult struct {
	ExperimentID       string            `json:"experiment_id"`
	Revision           string            `json:"revision"`
	ParticipantCount   int               `json:"participant_count"`
	Threshold          int               `json:"threshold"`
	InjectedFault      string            `json:"injected_fault"`
	InjectionPoint     string            `json:"injection_point"`
	ExpectedInvariant  string            `json:"expected_invariant"`
	TerminalResult     string            `json:"terminal_result"`
	TotalDurationMS    int64             `json:"total_duration_ms"`
	PhaseDurationMS    map[string]int64  `json:"phase_duration_ms"`
	RetryCount         int               `json:"retry_count"`
	TimeoutCount       int               `json:"timeout_count"`
	DuplicateCount     int               `json:"duplicate_count"`
	StaleMessageCount  int               `json:"stale_message_count"`
	DroppedCount       int               `json:"dropped_count"`
	DelayedCount       int               `json:"delayed_count"`
	RecoveryDurationMS *int64            `json:"recovery_duration_ms,omitempty"`
	Participants       []protocol.Status `json:"participants"`
	Unreachable        []string          `json:"unreachable,omitempty"`
}

func runExperiment(args []string) error {
	flags := flag.NewFlagSet("experiment", flag.ContinueOnError)
	scenario := flags.String("scenario", "", "experiment ID E0 through E6")
	deadline := flags.Duration("phase-deadline", 300*time.Millisecond, "SHARE_EXCHANGE deadline for incomplete scenarios")
	format := flags.String("format", "json", "output format: json or text")
	outputPath := flags.String("output", "", "optional path for a JSON result file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *scenario < "E0" || *scenario > "E6" || len(*scenario) != 2 {
		return errors.New("require --scenario E0 through E6")
	}
	if *deadline <= 0 {
		return errors.New("phase deadline must be positive")
	}
	if *format != "json" && *format != "text" {
		return errors.New("format must be json or text")
	}
	result, err := executeExperiment(*scenario, *deadline)
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if *outputPath != "" {
		if err := os.MkdirAll(filepath.Dir(*outputPath), 0755); err != nil {
			return fmt.Errorf("create result directory: %w", err)
		}
		if err := os.WriteFile(*outputPath, encoded, 0600); err != nil {
			return fmt.Errorf("write experiment result: %w", err)
		}
	}
	if *format == "text" {
		fmt.Printf("%s: %s in %d ms (%d participants, threshold %d)\n", result.ExperimentID, result.TerminalResult,
			result.TotalDurationMS, result.ParticipantCount, result.Threshold)
		fmt.Printf("fault: %s at %s\n", result.InjectedFault, result.InjectionPoint)
		fmt.Printf("retries=%d duplicates=%d stale=%d dropped=%d delayed=%d timeouts=%d\n",
			result.RetryCount, result.DuplicateCount, result.StaleMessageCount, result.DroppedCount,
			result.DelayedCount, result.TimeoutCount)
		if result.RecoveryDurationMS != nil {
			fmt.Printf("recovery=%d ms\n", *result.RecoveryDurationMS)
		}
		for _, status := range result.Participants {
			fmt.Printf("%s: %s, shares %d/%d\n", status.ParticipantID, status.Phase, status.Received, status.Expected)
		}
		for _, id := range result.Unreachable {
			fmt.Printf("%s: unreachable\n", id)
		}
		return nil
	}
	_, err = os.Stdout.Write(encoded)
	return err
}

func executeExperiment(scenario string, deadline time.Duration) (experimentResult, error) {
	meta := map[string][3]string{
		"E0": {"none", "normal delivery", "all participants finalize"},
		"E1": {"duplicate SHARE", "p2 to p1 after first delivery", "duplicate makes no additional transition"},
		"E2": {"stale SHARE", "old round p2 to p1 before normal delivery", "stale message cannot mutate state"},
		"E3": {"process crash and restart", "p1 after first peer SHARE", "replay preserves state and duplicate is idempotent"},
		"E4": {"one unavailable participant", "p4 after begin", "no finalization without required shares"},
		"E5": {"2:2 partition", "cross-group SHARE delivery", "no participant finalizes below threshold"},
		"E6": {"slow participant", "p4 outbound SHARE past deadline", "timeout is terminal and late SHARE cannot mutate state"},
	}[scenario]
	result := experimentResult{
		ExperimentID: scenario, Revision: buildRevision(), ParticipantCount: 4, Threshold: 3,
		InjectedFault: meta[0], InjectionPoint: meta[1], ExpectedInvariant: meta[2],
		PhaseDurationMS: make(map[string]int64),
	}
	executable, err := os.Executable()
	if err != nil {
		return result, err
	}
	stateDir, err := os.MkdirTemp("", "dkgctl-experiment-")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(stateDir)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second+deadline)
	defer cancel()
	client := transport.TCP{Timeout: time.Second}
	ids := []string{"p1", "p2", "p3", "p4"}
	processes := make([]participantProcess, 0, len(ids))
	defer func() { stopProcesses(processes) }()
	addresses := make(map[string]string, len(ids))
	for _, id := range ids {
		process, err := startExperimentParticipant(executable, id, filepath.Join(stateDir, id+".wal"))
		if err != nil {
			return result, err
		}
		processes = append(processes, process)
		addresses[id] = process.address
		if err := waitReady(ctx, client, process.address); err != nil {
			return result, fmt.Errorf("wait for %s: %w", id, err)
		}
	}
	config := protocol.Config{SessionID: "experiment-" + scenario, Epoch: 1, Round: 1, Threshold: 3, Participants: ids}
	ceremonyStart := time.Now()
	var outboxes []protocol.Message
	for _, id := range ids {
		response, err := client.Call(ctx, addresses[id], api.Request{Operation: "begin", Config: config})
		if err != nil {
			return result, fmt.Errorf("begin %s: %w", id, err)
		}
		outboxes = append(outboxes, response.Outbound...)
	}
	shareStart := time.Now()
	result.PhaseDurationMS["initialization"] = shareStart.Sub(ceremonyStart).Milliseconds()

	if scenario == "E2" {
		stale := findShare(outboxes, "p2", "p1")
		stale.Round = 0
		stale.MessageID = stale.LogicalID()
		before, err := experimentStatus(ctx, client, addresses["p1"])
		if err != nil {
			return result, err
		}
		if err := client.Send(ctx, addresses["p1"], stale); err == nil {
			return result, errors.New("stale SHARE was accepted")
		}
		after, err := experimentStatus(ctx, client, addresses["p1"])
		if err != nil || !reflect.DeepEqual(after, before) {
			return result, fmt.Errorf("stale SHARE changed state: before=%+v after=%+v err=%v", before, after, err)
		}
		result.StaleMessageCount++
	}
	if scenario == "E3" {
		first := findShare(outboxes, "p2", "p1")
		if err := client.Send(ctx, addresses["p1"], first); err != nil {
			return result, err
		}
		before, err := experimentStatus(ctx, client, addresses["p1"])
		if err != nil {
			return result, err
		}
		if before.Phase != protocol.PhaseShareExchange || before.Received != 2 {
			return result, fmt.Errorf("wrong pre-crash state: %+v", before)
		}
		if err := killExperimentParticipant(&processes[0]); err != nil {
			return result, err
		}
		restartAt := time.Now()
		processes[0], err = startExperimentParticipant(executable, "p1", filepath.Join(stateDir, "p1.wal"))
		if err != nil {
			return result, err
		}
		addresses["p1"] = processes[0].address
		if err := waitReady(ctx, client, addresses["p1"]); err != nil {
			return result, err
		}
		after, err := experimentStatus(ctx, client, addresses["p1"])
		if err != nil || !reflect.DeepEqual(after, before) {
			return result, fmt.Errorf("replay mismatch: before=%+v after=%+v err=%v", before, after, err)
		}
		elapsed := time.Since(restartAt).Milliseconds()
		result.RecoveryDurationMS = &elapsed
		result.RetryCount++
		result.DuplicateCount++ // The full delivery schedule retries p2 to p1.
	}
	if scenario == "E4" {
		if err := killExperimentParticipant(&processes[3]); err != nil {
			return result, err
		}
		result.Unreachable = append(result.Unreachable, "p4")
	}

	var held []protocol.Message
	for _, message := range outboxes {
		if scenario == "E4" && (message.From == "p4" || message.To == "p4") {
			result.DroppedCount++
			continue
		}
		if scenario == "E5" && (inFirstPartition(message.From) != inFirstPartition(message.To)) {
			result.DroppedCount++
			continue
		}
		if scenario == "E6" && message.From == "p4" {
			held = append(held, message)
			result.DelayedCount++
			continue
		}
		if scenario == "E1" && message.From == "p2" && message.To == "p1" {
			if err := client.Send(ctx, addresses[message.To], message); err != nil {
				return result, err
			}
			before, err := experimentStatus(ctx, client, addresses[message.To])
			if err != nil {
				return result, err
			}
			if err := client.Send(ctx, addresses[message.To], message); err != nil {
				return result, err
			}
			after, err := experimentStatus(ctx, client, addresses[message.To])
			if err != nil || !reflect.DeepEqual(after, before) {
				return result, fmt.Errorf("duplicate changed state: before=%+v after=%+v err=%v", before, after, err)
			}
			result.DuplicateCount++
			continue
		}
		if err := client.Send(ctx, addresses[message.To], message); err != nil {
			return result, fmt.Errorf("deliver %s to %s: %w", message.From, message.To, err)
		}
	}
	result.PhaseDurationMS["share_exchange"] = time.Since(shareStart).Milliseconds()
	if scenario == "E4" || scenario == "E5" || scenario == "E6" {
		if remaining := time.Until(shareStart.Add(deadline)); remaining > 0 {
			time.Sleep(remaining)
		}
		for _, id := range ids {
			if id == "p4" && scenario == "E4" {
				continue
			}
			if _, err := client.Call(ctx, addresses[id], api.Request{Operation: "timeout"}); err != nil {
				return result, fmt.Errorf("timeout %s: %w", id, err)
			}
			result.TimeoutCount++
		}
		result.PhaseDurationMS["share_exchange"] = time.Since(shareStart).Milliseconds()
		if scenario == "E6" {
			for _, message := range held {
				before, err := experimentStatus(ctx, client, addresses[message.To])
				if err != nil {
					return result, err
				}
				if err := client.Send(ctx, addresses[message.To], message); err == nil {
					return result, fmt.Errorf("late SHARE from %s was accepted", message.From)
				}
				after, err := experimentStatus(ctx, client, addresses[message.To])
				if err != nil || !reflect.DeepEqual(after, before) {
					return result, fmt.Errorf("late SHARE changed state: before=%+v after=%+v err=%v", before, after, err)
				}
				result.StaleMessageCount++
			}
		}
		result.TerminalResult = "timed_out"
	} else {
		verifyStart := time.Now()
		for _, id := range ids {
			status, err := experimentStatus(ctx, client, addresses[id])
			if err != nil || status.Phase != protocol.PhaseVerify {
				return result, fmt.Errorf("%s did not reach VERIFY: status=%+v err=%v", id, status, err)
			}
		}
		for _, id := range ids {
			if _, err := client.Call(ctx, addresses[id], api.Request{Operation: "finalize"}); err != nil {
				return result, fmt.Errorf("finalize %s: %w", id, err)
			}
		}
		result.PhaseDurationMS["verify"] = time.Since(verifyStart).Milliseconds()
		result.TerminalResult = "completed"
	}
	for _, id := range ids {
		if id == "p4" && scenario == "E4" {
			continue
		}
		status, err := experimentStatus(ctx, client, addresses[id])
		if err != nil {
			return result, err
		}
		if result.TerminalResult == "completed" && status.Phase != protocol.PhaseFinalize {
			return result, fmt.Errorf("%s failed to finalize: %+v", id, status)
		}
		if result.TerminalResult == "timed_out" && status.Phase != protocol.PhaseTimedOut {
			return result, fmt.Errorf("%s crossed timeout boundary: %+v", id, status)
		}
		result.Participants = append(result.Participants, status)
	}
	result.TotalDurationMS = time.Since(ceremonyStart).Milliseconds()
	return result, nil
}

func inFirstPartition(id string) bool {
	return id == "p1" || id == "p2"
}

func startExperimentParticipant(executable, id, stateFile string) (participantProcess, error) {
	address, err := unusedAddress()
	if err != nil {
		return participantProcess{}, err
	}
	command := exec.Command(executable, "participant", "--id", id, "--listen", address, "--state-file", stateFile)
	command.Stdout = os.Stderr
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		return participantProcess{}, fmt.Errorf("start %s: %w", id, err)
	}
	return participantProcess{id: id, address: address, command: command}, nil
}

func killExperimentParticipant(process *participantProcess) error {
	if process.command == nil {
		return errors.New("participant already stopped")
	}
	if err := process.command.Process.Kill(); err != nil {
		return err
	}
	if err := process.command.Wait(); err == nil {
		return errors.New("killed participant exited successfully")
	}
	process.command = nil
	return nil
}

func findShare(messages []protocol.Message, from, to string) protocol.Message {
	for _, message := range messages {
		if message.From == from && message.To == to {
			return message
		}
	}
	return protocol.Message{}
}

func experimentStatus(ctx context.Context, client transport.TCP, address string) (protocol.Status, error) {
	response, err := client.Call(ctx, address, api.Request{Operation: "status"})
	if err != nil {
		return protocol.Status{}, err
	}
	return response.Status, nil
}

func buildRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	revision := "unknown"
	modified := false
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			revision = setting.Value
		}
		if setting.Key == "vcs.modified" && setting.Value == "true" {
			modified = true
		}
	}
	if modified {
		return revision + "-dirty"
	}
	return revision
}
