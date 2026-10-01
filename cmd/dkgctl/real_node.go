package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/api"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/cryptoadapter"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/realexperiment"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/transport"
)

type realTCPNode struct {
	address string
	command *exec.Cmd
	stage   string
	tls     *tls.Config
}

func (n *realTCPNode) call(request api.Request) (api.Response, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	response, err := (transport.TCP{Timeout: 3 * time.Second, TLSConfig: n.tls}).Call(ctx, n.address, request)
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
	var result realexperiment.Result
	var err error
	if *scenario == "E3" {
		stateDir, err := os.MkdirTemp("", "dkgctl-real-e3-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(stateDir)
		result, err = runRealCeremony(realCeremonyOptions{Journal: filepath.Join(stateDir, "ceremony.jsonl"), MaxAttempts: 2, InjectCrash: true})
	} else {
		nodes, processes, startErr := startRealProcessNodes()
		if startErr != nil {
			return startErr
		}
		defer stopProcesses(processes)
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
	credentials, err := newLocalRealTLSCredentials()
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
			os.RemoveAll(credentials.dir)
			return nil, nil, err
		}
		command := exec.Command(executable, "real-participant", "--id", id, "--index", fmt.Sprint(i), "--listen", address,
			"--tls-cert", credentials.serverCert[i], "--tls-key", credentials.serverKey[i], "--tls-client-ca", credentials.caCert)
		command.Stdout, command.Stderr = os.Stderr, os.Stderr
		if err := command.Start(); err != nil {
			stopProcesses(processes)
			os.RemoveAll(credentials.dir)
			return nil, nil, err
		}
		process := participantProcess{id: id, address: address, command: command}
		if i == 0 {
			process.cleanupDir = credentials.dir
		}
		processes = append(processes, process)
		nodes = append(nodes, &realTCPNode{address: address, command: command, stage: "INIT", tls: credentials.clientConfig(id)})
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
