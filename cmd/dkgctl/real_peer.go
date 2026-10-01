package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/drand/kyber/share/dkg"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/api"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/cryptoadapter"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/realexperiment"
)

type peerRunResult struct {
	Revision           string                             `json:"revision"`
	Topology           string                             `json:"topology"`
	Transport          string                             `json:"transport"`
	SessionNonceSHA256 string                             `json:"session_nonce_sha256"`
	Participants       int                                `json:"participants"`
	Threshold          int                                `json:"threshold"`
	FinalizedCount     int                                `json:"finalized_count"`
	GroupKeyAgreement  string                             `json:"group_key_agreement"`
	DurationMS         int64                              `json:"duration_ms"`
	ParticipantResults []realexperiment.ParticipantResult `json:"participant_results"`
}

func runRealPeer(args []string) error {
	flags := flag.NewFlagSet("real-p2p-run", flag.ContinueOnError)
	output := flags.String("output", "", "optional JSON result file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	started := time.Now()
	nodes, processes, err := startRealProcessNodesWithMode(true)
	if err != nil {
		return err
	}
	defer stopProcesses(processes)
	identities := make([]cryptoadapter.KyberIdentity, len(nodes))
	for i, node := range nodes {
		identities[i], err = node.Identity()
		if err != nil {
			return err
		}
	}
	nonce := dkg.GetNonce()
	for i, node := range nodes {
		peers := make(map[string]string, len(nodes)-1)
		for j, process := range processes {
			if i != j {
				peers[process.id] = process.address
			}
		}
		if _, err := node.(*realTCPNode).call(api.Request{Operation: "real-configure",
			RealConfig: &api.RealConfig{Identities: identities, Threshold: 3, Nonce: nonce, Peers: peers}}); err != nil {
			return fmt.Errorf("configure %s: %w", processes[i].id, err)
		}
	}
	for i, node := range nodes {
		if _, err := node.(*realTCPNode).call(api.Request{Operation: "real-p2p-start"}); err != nil {
			return fmt.Errorf("start %s: %w", processes[i].id, err)
		}
	}
	deadline := time.Now().Add(35 * time.Second)
	for {
		finished := 0
		for i, node := range nodes {
			status, err := node.(*realTCPNode).call(api.Request{Operation: "real-status"})
			if err != nil {
				return fmt.Errorf("status %s: %w", processes[i].id, err)
			}
			if status.PeerError != "" {
				return fmt.Errorf("%s peer ceremony: %s", processes[i].id, status.PeerError)
			}
			if !status.PeerRunning && status.RealStage == "FINALIZE" {
				finished++
			}
		}
		if finished == len(nodes) {
			break
		}
		if time.Now().After(deadline) {
			return errors.New("timed out waiting for peer ceremony")
		}
		time.Sleep(20 * time.Millisecond)
	}
	hash := sha256.Sum256(nonce)
	result := peerRunResult{Revision: buildRevision(), Topology: "participant-to-participant",
		Transport: "mutual TLS over TCP, JSON RPC", SessionNonceSHA256: hex.EncodeToString(hash[:]),
		Participants: len(nodes), Threshold: 3, GroupKeyAgreement: "consistent"}
	var common []byte
	for i, node := range nodes {
		public, err := node.PublicResult()
		if err != nil {
			return err
		}
		groupHash := sha256.Sum256(public.GroupPublic)
		result.ParticipantResults = append(result.ParticipantResults, realexperiment.ParticipantResult{
			ID: processes[i].id, Phase: node.Stage(), Qualified: public.Qualified,
			GroupKeySHA256: hex.EncodeToString(groupHash[:])})
		result.FinalizedCount++
		if common != nil && !bytes.Equal(common, public.GroupPublic) {
			result.GroupKeyAgreement = "divergent"
		}
		common = public.GroupPublic
	}
	if result.GroupKeyAgreement != "consistent" {
		return errors.New("peer participants disagree on group public key")
	}
	result.DurationMS = time.Since(started).Milliseconds()
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
