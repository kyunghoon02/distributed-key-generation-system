package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/kyunghoon02/distributed-key-generation-system/internal/api"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/cryptoadapter"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/participant"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/protocol"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/transport"
)

type participantProcess struct {
	id      string
	address string
	command *exec.Cmd
}

type runResult struct {
	SessionID    string            `json:"session_id"`
	Participants []protocol.Status `json:"participants"`
	Finalized    bool              `json:"finalized"`
}

func main() {
	var err error
	switch {
	case len(os.Args) > 1 && os.Args[1] == "participant":
		err = runParticipant(os.Args[2:])
	case len(os.Args) > 1 && os.Args[1] == "run":
		err = run(os.Args[2:])
	default:
		fmt.Fprintln(os.Stderr, "usage: dkgctl run [--participants N] [--threshold T]")
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "dkgctl:", err)
		os.Exit(1)
	}
}

func runParticipant(args []string) error {
	flags := flag.NewFlagSet("participant", flag.ContinueOnError)
	id := flags.String("id", "", "stable participant ID")
	address := flags.String("listen", "127.0.0.1:0", "TCP listen address")
	stateFile := flags.String("state-file", "", "durable participant state file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *id == "" {
		return errors.New("-id is required")
	}
	listener, err := net.Listen("tcp", *address)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer listener.Close()
	return participantServer(*id, *stateFile, listener)
}

func participantServer(id, stateFile string, listener net.Listener) error {
	if stateFile == "" {
		return participant.NewServer(id, cryptoadapter.Mock{}).Serve(listener)
	}
	server, err := participant.NewDurableServer(id, cryptoadapter.Mock{}, stateFile)
	if err != nil {
		return fmt.Errorf("recover participant state: %w", err)
	}
	defer server.Close()
	return server.Serve(listener)
}

func run(args []string) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	participantCount := flags.Int("participants", 4, "number of local participant processes")
	threshold := flags.Int("threshold", 3, "mock ceremony threshold")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *participantCount < 1 || *threshold < 1 || *threshold > *participantCount {
		return errors.New("require participants >= threshold >= 1")
	}

	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate dkgctl executable: %w", err)
	}
	stateDir, err := os.MkdirTemp("", "dkgctl-state-")
	if err != nil {
		return fmt.Errorf("create temporary state directory: %w", err)
	}
	defer os.RemoveAll(stateDir)
	processes := make([]participantProcess, 0, *participantCount)
	for i := 0; i < *participantCount; i++ {
		id := fmt.Sprintf("p%d", i+1)
		address, err := unusedAddress()
		if err != nil {
			stopProcesses(processes)
			return err
		}
		command := exec.Command(executable, "participant", "--id", id, "--listen", address,
			"--state-file", filepath.Join(stateDir, id+".wal"))
		command.Stdout = os.Stderr
		command.Stderr = os.Stderr
		if err := command.Start(); err != nil {
			stopProcesses(processes)
			return fmt.Errorf("start %s: %w", id, err)
		}
		processes = append(processes, participantProcess{id: id, address: address, command: command})
	}
	defer func() { stopProcesses(processes) }()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client := transport.TCP{Timeout: 2 * time.Second}
	for _, process := range processes {
		if err := waitReady(ctx, client, process.address); err != nil {
			return fmt.Errorf("wait for %s: %w", process.id, err)
		}
	}

	ids := make([]string, 0, len(processes))
	addresses := make(map[string]string, len(processes))
	for _, process := range processes {
		ids = append(ids, process.id)
		addresses[process.id] = process.address
	}
	sort.Strings(ids)
	config := protocol.Config{
		SessionID:    "m0-normal-run",
		Epoch:        1,
		Round:        1,
		Threshold:    *threshold,
		Participants: ids,
	}
	outboxes := make([]protocol.Message, 0, *participantCount*(*participantCount-1))
	for _, process := range processes {
		response, err := client.Call(ctx, process.address, api.Request{Operation: "begin", Config: config})
		if err != nil {
			return fmt.Errorf("begin at %s: %w", process.id, err)
		}
		outboxes = append(outboxes, response.Outbound...)
	}
	for _, message := range outboxes {
		address, ok := addresses[message.To]
		if !ok {
			return fmt.Errorf("unknown recipient %q", message.To)
		}
		if err := client.Send(ctx, address, message); err != nil {
			return fmt.Errorf("deliver %s to %s: %w", message.From, message.To, err)
		}
	}

	result := runResult{SessionID: config.SessionID, Finalized: true}
	for _, process := range processes {
		if _, err := client.Call(ctx, process.address, api.Request{Operation: "finalize"}); err != nil {
			return fmt.Errorf("finalize at %s: %w", process.id, err)
		}
		response, err := client.Call(ctx, process.address, api.Request{Operation: "status"})
		if err != nil {
			return fmt.Errorf("status at %s: %w", process.id, err)
		}
		result.Participants = append(result.Participants, response.Status)
		if response.Status.Phase != protocol.PhaseFinalize {
			result.Finalized = false
		}
	}
	if !result.Finalized {
		return errors.New("ceremony did not finalize at every participant")
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func unusedAddress() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("allocate local port: %w", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		return "", fmt.Errorf("release local port: %w", err)
	}
	return address, nil
}

func waitReady(ctx context.Context, client transport.TCP, address string) error {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, err := client.Call(ctx, address, api.Request{Operation: "status"})
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func stopProcesses(processes []participantProcess) {
	for _, process := range processes {
		if process.command.Process != nil {
			_ = process.command.Process.Kill()
			_ = process.command.Wait()
		}
	}
}
