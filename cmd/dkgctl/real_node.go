package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/drand/kyber/share/dkg"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/api"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/cryptoadapter"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/realexperiment"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/transport"
)

type realTCPNode struct {
	address string
	command *exec.Cmd
	stage   string
}

func (n *realTCPNode) call(request api.Request) (api.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	response, err := (transport.TCP{Timeout: 3 * time.Second}).Call(ctx, n.address, request)
	if response.RealStage != "" {
		n.stage = response.RealStage
	}
	return response, err
}

func (n *realTCPNode) Identity() (cryptoadapter.KyberIdentity, error) {
	response, err := n.call(api.Request{Operation: "real-identity"})
	if err != nil {
		return cryptoadapter.KyberIdentity{}, err
	}
	if response.RealIdentity == nil {
		return cryptoadapter.KyberIdentity{}, errors.New("missing real identity")
	}
	return *response.RealIdentity, nil
}

func (n *realTCPNode) Configure(ids []cryptoadapter.KyberIdentity, threshold int, nonce []byte) error {
	_, err := n.call(api.Request{Operation: "real-configure", RealConfig: &api.RealConfig{Identities: ids, Threshold: threshold, Nonce: nonce}})
	return err
}

func (n *realTCPNode) Deals() (cryptoadapter.KyberPacket, error) {
	response, err := n.call(api.Request{Operation: "real-deals"})
	if err != nil {
		return cryptoadapter.KyberPacket{}, err
	}
	if response.RealPacket == nil {
		return cryptoadapter.KyberPacket{}, errors.New("missing real deal")
	}
	return *response.RealPacket, nil
}

func (n *realTCPNode) Accept(packet cryptoadapter.KyberPacket) (bool, error) {
	response, err := n.call(api.Request{Operation: "real-accept", RealPacket: &packet})
	return response.RealDuplicate, err
}

func (n *realTCPNode) ProcessDeals() (*cryptoadapter.KyberPacket, error) {
	response, err := n.call(api.Request{Operation: "real-process-deals"})
	return response.RealPacket, err
}

func (n *realTCPNode) ProcessResponses() (*cryptoadapter.KyberPacket, error) {
	response, err := n.call(api.Request{Operation: "real-process-responses"})
	return response.RealPacket, err
}

func (n *realTCPNode) ProcessJustifications() error {
	_, err := n.call(api.Request{Operation: "real-process-justifications"})
	return err
}

func (n *realTCPNode) Stage() string { return n.stage }

func (n *realTCPNode) Timeout() error {
	_, err := n.call(api.Request{Operation: "real-timeout"})
	return err
}

func (n *realTCPNode) Abort() error {
	_, err := n.call(api.Request{Operation: "real-abort"})
	return err
}

func (n *realTCPNode) PublicResult() (cryptoadapter.KyberPublicResult, error) {
	response, err := n.call(api.Request{Operation: "real-result"})
	if err != nil {
		return cryptoadapter.KyberPublicResult{}, err
	}
	if response.RealResult == nil {
		return cryptoadapter.KyberPublicResult{}, errors.New("missing real result")
	}
	return *response.RealResult, nil
}

func (n *realTCPNode) Crash() error {
	if n.command == nil || n.command.Process == nil {
		return errors.New("real node has no process")
	}
	if err := n.command.Process.Kill(); err != nil {
		return err
	}
	if err := n.command.Wait(); err == nil {
		return errors.New("killed process exited successfully")
	}
	n.command.Process = nil
	n.stage = "UNREACHABLE"
	return nil
}

func runRealProcessExperiment(args []string) error {
	flags := flag.NewFlagSet("real-run", flag.ContinueOnError)
	scenario := flags.String("scenario", "E0", "fault schedule E0 through E6")
	output := flags.String("output", "", "optional JSON result file")
	hold := flags.Duration("hold-duration", 100*time.Millisecond, "E6 hold time after terminal state")
	if err := flags.Parse(args); err != nil {
		return err
	}
	nodes, processes, err := startRealProcessNodes()
	if err != nil {
		return err
	}
	defer stopProcesses(processes)
	var result realexperiment.Result
	if *scenario == "E3" {
		result, err = runRealFreshSessionRecovery(nodes)
	} else {
		result, err = realexperiment.RunWithNodesHold(*scenario, nodes, *hold)
	}
	if err != nil {
		return err
	}
	result.Revision = buildRevision()
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if *output != "" {
		if err := os.WriteFile(*output, data, 0o644); err != nil {
			return err
		}
	}
	_, err = os.Stdout.Write(data)
	return err
}

func startRealProcessNodes() ([]realexperiment.Node, []participantProcess, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, nil, err
	}
	processes := make([]participantProcess, 0, 4)
	nodes := make([]realexperiment.Node, 0, 4)
	for i := 0; i < 4; i++ {
		id := fmt.Sprintf("p%d", i+1)
		address, err := unusedAddress()
		if err != nil {
			stopProcesses(processes)
			return nil, nil, err
		}
		command := exec.Command(executable, "real-participant", "--id", id, "--index", fmt.Sprint(i), "--listen", address)
		command.Stdout, command.Stderr = os.Stderr, os.Stderr
		if err := command.Start(); err != nil {
			stopProcesses(processes)
			return nil, nil, err
		}
		processes = append(processes, participantProcess{id: id, address: address, command: command})
		nodes = append(nodes, &realTCPNode{address: address, command: command, stage: "INIT"})
	}
	for i, node := range nodes {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		for {
			_, err := node.Identity()
			if err == nil {
				break
			}
			if ctx.Err() != nil {
				cancel()
				stopProcesses(processes)
				return nil, nil, fmt.Errorf("wait for real participant %d: %w", i, err)
			}
			time.Sleep(20 * time.Millisecond)
		}
		cancel()
	}
	return nodes, processes, nil
}

// A Kyber DKG engine cannot be reconstructed from the mock WAL. On a real
// process crash, abort the old ceremony and start a new one with fresh node
// identities and nonce; never replay old deals into the new ceremony.
func runRealFreshSessionRecovery(nodes []realexperiment.Node) (realexperiment.Result, error) {
	started := time.Now()
	identities := make([]cryptoadapter.KyberIdentity, 4)
	for i, node := range nodes {
		identity, err := node.Identity()
		if err != nil {
			return realexperiment.Result{}, err
		}
		identities[i] = identity
	}
	nonce := dkg.GetNonce()
	abortedNonceHash := sha256.Sum256(nonce)
	for _, node := range nodes {
		if err := node.Configure(identities, 3, nonce); err != nil {
			return realexperiment.Result{}, err
		}
	}
	deals := make([]cryptoadapter.KyberPacket, 4)
	for i, node := range nodes {
		deal, err := node.Deals()
		if err != nil {
			return realexperiment.Result{}, err
		}
		deals[i] = deal
	}
	if _, err := nodes[0].Accept(deals[1]); err != nil {
		return realexperiment.Result{}, err
	}
	recoveryStarted := time.Now()
	if err := nodes[0].(interface{ Crash() error }).Crash(); err != nil {
		return realexperiment.Result{}, err
	}
	oldProcesses := make([]participantProcess, 0, 3)
	for _, node := range nodes[1:] {
		tcpNode := node.(*realTCPNode)
		oldProcesses = append(oldProcesses, participantProcess{command: tcpNode.command})
	}
	stopProcesses(oldProcesses)
	freshNodes, freshProcesses, err := startRealProcessNodes()
	if err != nil {
		return realexperiment.Result{}, err
	}
	defer stopProcesses(freshProcesses)
	result, err := realexperiment.RunWithNodes("E0", freshNodes)
	if err != nil {
		return realexperiment.Result{}, err
	}
	result.Scenario = "E3"
	result.AbortedNonceSHA256 = hex.EncodeToString(abortedNonceHash[:])
	result.Fault = "p1 crashed after receiving p2 deal; old ceremony aborted; fresh ceremony completed"
	result.InjectionPoint = "after first peer deal at p1"
	result.ExpectedInvariant = "aborted session is not resumed; fresh nonce and identities produce one agreed group key"
	result.TerminalResult = "completed_after_abort"
	result.RecoveryMode = "fresh_session_after_abort"
	result.AbortedSessions = 1
	result.RestartedProcesses = 4
	result.RetryCount = 1
	result.FreshRunDurationMS = result.TotalDurationMS
	result.TotalDurationMS = time.Since(started).Milliseconds()
	recoveryDuration := time.Since(recoveryStarted).Milliseconds()
	result.RecoveryDurationMS = &recoveryDuration
	return result, nil
}
