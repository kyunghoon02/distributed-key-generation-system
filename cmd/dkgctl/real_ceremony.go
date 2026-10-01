package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/drand/kyber/share/dkg"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/cryptoadapter"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/realexperiment"
)

type ceremonyEvent struct {
	Attempt     int       `json:"attempt"`
	NonceSHA256 string    `json:"nonce_sha256"`
	Status      string    `json:"status"`
	At          time.Time `json:"at"`
}

type realCeremonyOptions struct {
	Journal     string
	MaxAttempts int
	InjectCrash bool // E3 test schedule only.
}

func runRealCeremonyCommand(args []string) error {
	flags := flag.NewFlagSet("real-ceremony", flag.ContinueOnError)
	journal := flags.String("journal", filepath.Join(".dkgctl", "real-ceremony.jsonl"), "controller event journal (contains nonce hashes, no shares)")
	maxAttempts := flags.Int("max-attempts", 2, "maximum fresh ceremonies after process or RPC failure")
	if err := flags.Parse(args); err != nil {
		return err
	}
	result, err := runRealCeremony(realCeremonyOptions{Journal: *journal, MaxAttempts: *maxAttempts})
	if err != nil {
		return err
	}
	result.Revision = buildRevision()
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// runRealCeremony is the normal real-DKG process lifecycle. Every failed
// attempt is fenced by stopping all local processes before a fresh roster and
// nonce are generated. No Kyber private state is loaded from the journal.
func runRealCeremony(options realCeremonyOptions) (realexperiment.Result, error) {
	if options.Journal == "" || options.MaxAttempts < 1 {
		return realexperiment.Result{}, errors.New("journal and positive max-attempts are required")
	}
	started := time.Now()
	if err := os.MkdirAll(filepath.Dir(options.Journal), 0o700); err != nil {
		return realexperiment.Result{}, err
	}
	lock, err := os.OpenFile(options.Journal+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return realexperiment.Result{}, err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return realexperiment.Result{}, fmt.Errorf("real ceremony journal is in use: %w", err)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	events, err := readCeremonyJournal(options.Journal)
	if err != nil {
		return realexperiment.Result{}, err
	}
	used := make(map[string]bool, len(events))
	for _, event := range events {
		used[event.NonceSHA256] = true
	}
	aborted := 0
	var abortedHash string
	if len(events) > 0 && events[len(events)-1].Status == "started" {
		last := events[len(events)-1]
		if err := appendCeremonyEvent(options.Journal, ceremonyEvent{Attempt: last.Attempt, NonceSHA256: last.NonceSHA256, Status: "aborted", At: time.Now().UTC()}); err != nil {
			return realexperiment.Result{}, err
		}
		aborted++
		abortedHash = last.NonceSHA256
	}
	nextAttempt := 1
	if len(events) > 0 {
		nextAttempt = events[len(events)-1].Attempt + 1
	}
	var oldPacket *cryptoadapter.KyberPacket
	var recoveryStarted time.Time
	var lastErr error
	for retry := 0; retry < options.MaxAttempts; retry++ {
		nodes, processes, err := startRealProcessNodes()
		if err != nil {
			lastErr = err
			continue
		}
		nonce := dkg.GetNonce()
		hash := sha256.Sum256(nonce)
		nonceHash := hex.EncodeToString(hash[:])
		if used[nonceHash] {
			stopProcesses(processes)
			return realexperiment.Result{}, errors.New("reused DKG session nonce")
		}
		used[nonceHash] = true
		attempt := nextAttempt + retry
		if err := appendCeremonyEvent(options.Journal, ceremonyEvent{Attempt: attempt, NonceSHA256: nonceHash, Status: "started", At: time.Now().UTC()}); err != nil {
			stopProcesses(processes)
			return realexperiment.Result{}, err
		}
		runOptions := realexperiment.RunOptions{Nonce: nonce}
		if oldPacket != nil {
			runOptions.AfterConfigure = func(nodes []realexperiment.Node) error {
				if _, err := nodes[0].Accept(*oldPacket); err == nil {
					return errors.New("old-session packet accepted in fresh ceremony")
				}
				return nil
			}
		}
		if options.InjectCrash && retry == 0 {
			runOptions.AfterDelivery = func(from, to int, packet cryptoadapter.KyberPacket) error {
				if from != 1 || to != 0 || packet.Kind != "deal" {
					return nil
				}
				copyPacket := packet
				oldPacket = &copyPacket
				recoveryStarted = time.Now()
				if err := nodes[0].(interface{ Crash() error }).Crash(); err != nil {
					return err
				}
				return errors.New("injected p1 process crash after peer deal")
			}
		}
		result, runErr := realexperiment.RunWithNodesOptions("E0", nodes, runOptions)
		if runErr == nil && (result.TerminalResult != "completed" || result.FinalizedCount != 4 || result.GroupKeyAgreement != "consistent") {
			runErr = fmt.Errorf("ceremony did not finalize at all participants: %s", result.TerminalResult)
		}
		if runErr != nil && recoveryStarted.IsZero() {
			recoveryStarted = time.Now()
		}
		stopProcesses(processes)
		status := "finalized"
		if runErr != nil {
			status = "aborted"
		}
		if err := appendCeremonyEvent(options.Journal, ceremonyEvent{Attempt: attempt, NonceSHA256: nonceHash, Status: status, At: time.Now().UTC()}); err != nil {
			return realexperiment.Result{}, err
		}
		if runErr != nil {
			aborted++
			abortedHash = nonceHash
			lastErr = runErr
			continue
		}
		result.AbortedSessions = aborted
		result.AbortedNonceSHA256 = abortedHash
		result.RestartedProcesses = retry * 4
		result.RetryCount = retry
		result.FreshRunDurationMS = result.TotalDurationMS
		result.TotalDurationMS = time.Since(started).Milliseconds()
		if aborted > 0 {
			result.RecoveryMode = "fresh_session_after_abort"
			if !recoveryStarted.IsZero() {
				duration := time.Since(recoveryStarted).Milliseconds()
				result.RecoveryDurationMS = &duration
			}
		}
		if options.InjectCrash {
			result.Scenario = "E3"
			result.Fault = "p1 crashed after receiving p2 deal; old ceremony aborted; fresh ceremony completed"
			result.InjectionPoint = "after first peer deal at p1"
			result.ExpectedInvariant = "old packet rejected; fresh nonce and identities produce one agreed group key"
			result.TerminalResult = "completed_after_abort"
			if oldPacket != nil {
				result.StaleRejectedCount++
			}
		}
		return result, nil
	}
	return realexperiment.Result{}, fmt.Errorf("real ceremony exhausted %d attempts: %w", options.MaxAttempts, lastErr)
}

func readCeremonyJournal(path string) ([]ceremonyEvent, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var events []ceremonyEvent
	decoder := json.NewDecoder(file)
	for {
		var event ceremonyEvent
		if err := decoder.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				return events, nil
			}
			return nil, fmt.Errorf("decode real ceremony journal: %w", err)
		}
		hash, hashErr := hex.DecodeString(event.NonceSHA256)
		if event.Attempt < 1 || hashErr != nil || len(hash) != sha256.Size || (event.Status != "started" && event.Status != "aborted" && event.Status != "finalized") {
			return nil, errors.New("invalid real ceremony journal event")
		}
		if len(events) == 0 {
			if event.Status != "started" {
				return nil, errors.New("real ceremony journal must start with a started event")
			}
		} else {
			previous := events[len(events)-1]
			if previous.Status == "started" {
				if event.Attempt != previous.Attempt || event.NonceSHA256 != previous.NonceSHA256 || event.Status == "started" {
					return nil, errors.New("invalid real ceremony journal transition")
				}
			} else if event.Status != "started" || event.Attempt != previous.Attempt+1 {
				return nil, errors.New("invalid real ceremony journal transition")
			}
		}
		events = append(events, event)
	}
}

func appendCeremonyEvent(path string, event ceremonyEvent) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := json.NewEncoder(file).Encode(event); err != nil {
		return err
	}
	return file.Sync()
}
