package realexperiment

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/drand/kyber/share/dkg"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/cryptoadapter"
)

type ParticipantResult struct {
	ID             string `json:"id"`
	Phase          string `json:"phase"`
	Qualified      int    `json:"qualified,omitempty"`
	GroupKeySHA256 string `json:"group_key_sha256,omitempty"`
	Error          string `json:"error,omitempty"`
}

type Result struct {
	Scenario           string              `json:"scenario"`
	Revision           string              `json:"revision"`
	Crypto             string              `json:"crypto"`
	Participants       int                 `json:"participants"`
	Threshold          int                 `json:"threshold"`
	SessionNonceSHA256 string              `json:"session_nonce_sha256"`
	AbortedNonceSHA256 string              `json:"aborted_nonce_sha256,omitempty"`
	Fault              string              `json:"fault"`
	InjectionPoint     string              `json:"injection_point"`
	ExpectedInvariant  string              `json:"expected_invariant"`
	TerminalResult     string              `json:"terminal_result"`
	TotalDurationMS    int64               `json:"total_duration_ms"`
	PhaseDurationMS    map[string]int64    `json:"phase_duration_ms"`
	RetryCount         int                 `json:"retry_count"`
	TimeoutCount       int                 `json:"timeout_count"`
	AbortCount         int                 `json:"abort_count"`
	DuplicateCount     int                 `json:"duplicate_count"`
	StaleRejectedCount int                 `json:"stale_rejected_count"`
	DroppedCount       int                 `json:"dropped_count"`
	DelayedCount       int                 `json:"delayed_count"`
	HoldDurationMS     int64               `json:"hold_duration_ms,omitempty"`
	LateRejectedCount  int                 `json:"late_rejected_count"`
	FinalizedCount     int                 `json:"finalized_count"`
	GroupKeyAgreement  string              `json:"group_key_agreement"`
	RecoveryMode       string              `json:"recovery_mode,omitempty"`
	AbortedSessions    int                 `json:"aborted_sessions,omitempty"`
	RestartedProcesses int                 `json:"restarted_processes,omitempty"`
	FreshRunDurationMS int64               `json:"fresh_run_duration_ms,omitempty"`
	RecoveryDurationMS *int64              `json:"recovery_duration_ms,omitempty"`
	ParticipantResults []ParticipantResult `json:"participant_results"`
}

type Node interface {
	Identity() (cryptoadapter.KyberIdentity, error)
	Configure([]cryptoadapter.KyberIdentity, int, []byte) error
	Deals() (cryptoadapter.KyberPacket, error)
	Accept(cryptoadapter.KyberPacket) (bool, error)
	ProcessDeals() (*cryptoadapter.KyberPacket, error)
	ProcessResponses() (*cryptoadapter.KyberPacket, error)
	ProcessJustifications() error
	Stage() string
	Timeout() error
	Abort() error
	PublicResult() (cryptoadapter.KyberPublicResult, error)
}

// Run executes a single memory-only Kyber DKG ceremony using a deterministic
// delivery schedule. The cryptographic payloads remain random. This is not the
// mock runtime's process/WAL recovery path.
func Run(scenario string) (Result, error) {
	nodes := make([]Node, 4)
	for i := range nodes {
		participant, err := cryptoadapter.NewKyberParticipant(fmt.Sprintf("p%d", i+1), uint32(i))
		if err != nil {
			return Result{}, err
		}
		nodes[i] = participant
	}
	return RunWithNodes(scenario, nodes)
}

// RunWithNodes uses the same scheduler against local adapters or TCP participant
// processes. Node implementations retain their secret state and return only
// public results to the controller.
func RunWithNodes(scenario string, nodes []Node) (Result, error) {
	return RunWithNodesHold(scenario, nodes, 100*time.Millisecond)
}

func RunWithNodesHold(scenario string, nodes []Node, hold time.Duration) (Result, error) {
	if len(nodes) != 4 {
		return Result{}, errors.New("real experiments require four nodes")
	}
	if hold < 0 {
		return Result{}, errors.New("negative packet hold duration")
	}
	faults := map[string]string{
		"E0": "none", "E1": "duplicate p2 deal to p1", "E2": "stale p2 deal to p1",
		"E4": "p4 unavailable", "E5": "p1,p2 isolated from p3,p4",
		"E6": "p4 deals held until recipients terminate, then held again before late delivery",
	}
	fault, ok := faults[scenario]
	if !ok {
		return Result{}, fmt.Errorf("unsupported real DKG scenario %q (E3 recovery requires durable crypto state)", scenario)
	}
	started := time.Now()
	invariants := map[string]string{
		"E0": "all four agree on one group public key",
		"E1": "duplicate deal changes no state and all four finalize",
		"E2": "stale-session deal rejected and all four finalize",
		"E4": "three responsive participants finalize with one group key",
		"E5": "neither two-node partition finalizes at threshold three",
		"E6": "late deal rejected after terminal state; three participants agree",
	}
	result := Result{Scenario: scenario, Crypto: "drand/kyber v1.3.2 Pedersen DKG, Ed25519",
		Participants: 4, Threshold: 3, Fault: fault, InjectionPoint: "after deal generation",
		ExpectedInvariant: invariants[scenario], GroupKeyAgreement: "not_observed",
		PhaseDurationMS: make(map[string]int64)}
	identities := make([]cryptoadapter.KyberIdentity, 4)
	for i := range nodes {
		var err error
		identities[i], err = nodes[i].Identity()
		if err != nil {
			return Result{}, err
		}
	}
	nonce := dkg.GetNonce()
	nonceHash := sha256.Sum256(nonce)
	result.SessionNonceSHA256 = hex.EncodeToString(nonceHash[:])
	for _, node := range nodes {
		if err := node.Configure(identities, 3, nonce); err != nil {
			return Result{}, err
		}
	}
	deals := make([]cryptoadapter.KyberPacket, len(nodes))
	for i, node := range nodes {
		var err error
		deals[i], err = node.Deals()
		if err != nil {
			return Result{}, err
		}
	}
	result.PhaseDurationMS["initialization_and_deal_generation"] = time.Since(started).Milliseconds()
	phaseStarted := time.Now()
	if scenario == "E4" {
		if crashable, ok := nodes[3].(interface{ Crash() error }); ok {
			if err := crashable.Crash(); err != nil {
				return Result{}, err
			}
		}
	}
	active := func(i int) bool { return scenario != "E4" || i != 3 }
	blocked := func(from, to int) bool {
		if !active(from) || !active(to) {
			return true
		}
		if scenario == "E5" && (from < 2) != (to < 2) {
			return true
		}
		if scenario == "E6" && from == 3 {
			return true
		}
		return false
	}
	deliver := func(from, to int, packet cryptoadapter.KyberPacket) error {
		if blocked(from, to) {
			result.DroppedCount++
			return nil
		}
		encoded, err := json.Marshal(packet)
		if err != nil {
			return err
		}
		var wire cryptoadapter.KyberPacket
		if err := json.Unmarshal(encoded, &wire); err != nil {
			return err
		}
		if _, err := nodes[to].Accept(wire); err != nil {
			return fmt.Errorf("%s %d -> %d: %w", packet.Kind, from, to, err)
		}
		return nil
	}
	for from, deal := range deals {
		for to := range nodes {
			if from == to {
				continue
			}
			if from == 1 && to == 0 && scenario == "E2" {
				stale := deal
				stale.SessionID = bytes.Repeat([]byte{0}, dkg.NonceLength)
				if _, err := nodes[to].Accept(stale); err == nil {
					return Result{}, errors.New("stale deal accepted")
				}
				result.StaleRejectedCount++
			}
			if err := deliver(from, to, deal); err != nil {
				return Result{}, err
			}
			if from == 1 && to == 0 && scenario == "E1" {
				duplicate, err := nodes[to].Accept(deal)
				if err != nil || !duplicate {
					return Result{}, errors.New("duplicate deal was not idempotent")
				}
				result.DuplicateCount++
			}
		}
	}
	result.PhaseDurationMS["deal_delivery"] = time.Since(phaseStarted).Milliseconds()
	phaseStarted = time.Now()
	responses := make([]*cryptoadapter.KyberPacket, len(nodes))
	issues := make([]string, len(nodes))
	for i, node := range nodes {
		if !active(i) {
			continue
		}
		var err error
		responses[i], err = node.ProcessDeals()
		if err != nil {
			issues[i] = err.Error()
		}
	}
	for from, response := range responses {
		if response == nil {
			continue
		}
		for to := range nodes {
			if from == to || issues[to] != "" {
				continue
			}
			if err := deliver(from, to, *response); err != nil {
				return Result{}, err
			}
		}
	}
	justifications := make([]*cryptoadapter.KyberPacket, len(nodes))
	for i, node := range nodes {
		if !active(i) || issues[i] != "" {
			continue
		}
		var err error
		justifications[i], err = node.ProcessResponses()
		if err != nil {
			issues[i] = err.Error()
		}
	}
	for from, justification := range justifications {
		if justification == nil {
			continue
		}
		for to := range nodes {
			if from == to || issues[to] != "" {
				continue
			}
			if err := deliver(from, to, *justification); err != nil {
				return Result{}, err
			}
		}
	}
	for i, node := range nodes {
		if node.Stage() == "COLLECT_JUSTIFICATIONS" && issues[i] == "" {
			if err := node.ProcessJustifications(); err != nil {
				issues[i] = err.Error()
			}
		}
	}
	result.PhaseDurationMS["response_and_justification"] = time.Since(phaseStarted).Milliseconds()
	var common []byte
	for i, node := range nodes {
		if !active(i) {
			result.ParticipantResults = append(result.ParticipantResults, ParticipantResult{ID: identities[i].ID, Phase: "UNREACHABLE"})
			continue
		}
		entry := ParticipantResult{ID: identities[i].ID, Phase: node.Stage(), Error: issues[i]}
		if node.Stage() == "FINALIZE" {
			public, err := node.PublicResult()
			if err != nil {
				return Result{}, err
			}
			entry.Qualified = public.Qualified
			hash := sha256.Sum256(public.GroupPublic)
			entry.GroupKeySHA256 = hex.EncodeToString(hash[:])
			result.FinalizedCount++
			if common == nil {
				common = public.GroupPublic
				result.GroupKeyAgreement = "consistent"
			} else if !bytes.Equal(common, public.GroupPublic) {
				result.GroupKeyAgreement = "divergent"
			}
		} else {
			var err error
			if issues[i] != "" {
				result.AbortCount++
				err = node.Abort()
			} else {
				result.TimeoutCount++
				err = node.Timeout()
			}
			if err != nil {
				return Result{}, err
			}
			entry.Phase = node.Stage()
		}
		result.ParticipantResults = append(result.ParticipantResults, entry)
	}
	if result.FinalizedCount >= result.Threshold && result.GroupKeyAgreement == "consistent" {
		result.TerminalResult = "completed"
	} else if result.AbortCount > 0 {
		result.TerminalResult = "aborted"
	} else {
		result.TerminalResult = "timed_out"
	}
	if scenario == "E6" {
		holdStarted := time.Now()
		time.Sleep(hold)
		result.HoldDurationMS = time.Since(holdStarted).Milliseconds()
		result.PhaseDurationMS["held_after_terminal"] = result.HoldDurationMS
		for to := 0; to < 3; to++ {
			if _, err := nodes[to].Accept(deals[3]); err == nil {
				return Result{}, errors.New("late deal accepted")
			}
			result.LateRejectedCount++
			result.DelayedCount++
		}
	}
	result.TotalDurationMS = time.Since(started).Milliseconds()
	return result, nil
}
