package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/bytemare/frost"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/agentpayment"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/api"
	"github.com/kyunghoon02/distributed-key-generation-system/internal/cryptoadapter"
)

type agentPaymentResult struct {
	Revision       string                     `json:"revision"`
	Mode           string                     `json:"mode"`
	DKGTopology    string                     `json:"dkg_topology"`
	Threshold      int                        `json:"threshold"`
	SignerIDs      []string                   `json:"signer_ids"`
	GroupKeySHA256 string                     `json:"group_key_sha256"`
	Authorization  agentpayment.Authorization `json:"authorization"`
	Receipt        agentpayment.Receipt       `json:"receipt"`
	DurationMS     int64                      `json:"duration_ms"`
}

func runAgentPayment(args []string) error {
	flags := flag.NewFlagSet("agent-payment-demo", flag.ContinueOnError)
	amount := flags.Int64("amount-minor", 1200, "sandbox amount in USD cents")
	merchant := flags.String("merchant", "paid-api.example", "merchant identifier")
	signerCount := flags.Int("signer-count", 3, "number of available signers for the sandbox attempt")
	tamperAfterCommit := flags.Bool("tamper-after-commit", false, "test rejection when amount changes after commitments")
	offlineP4 := flags.Bool("offline-p4", false, "stop p4 after DKG to demonstrate 3-of-4 signing")
	output := flags.String("output", "", "optional JSON result file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *signerCount < 1 || *signerCount > 3 {
		return errors.New("signer-count must be between 1 and 3")
	}
	started := time.Now()
	nodes, processes, err := startRealProcessNodesWithMode(true)
	if err != nil {
		return err
	}
	defer stopProcesses(processes)
	_, publics, err := completeLocalPeerDKG(nodes, processes, "agent-payment-authority", 1)
	if err != nil {
		return err
	}
	if *offlineP4 {
		if err := nodes[3].(*realTCPNode).Crash(); err != nil {
			return err
		}
	}
	policy := agentpayment.Policy{BudgetID: "demo-budget", AgentID: "research-agent",
		KeyID: "agent-payment-authority", KeyVersion: 1, Currency: "USD",
		AllowedMerchantIDs: []string{"paid-api.example"}, MaxPerPaymentMinor: 5000,
		MaxTotalMinor: 10000, ValidUntil: time.Now().Add(time.Hour).UTC()}
	request := agentpayment.Request{RequestID: "demo-payment-1", AgentID: policy.AgentID,
		MerchantID: *merchant, BudgetID: policy.BudgetID, KeyID: policy.KeyID,
		KeyVersion: policy.KeyVersion, AmountMinor: *amount, Currency: policy.Currency,
		ExpiresAt: time.Now().Add(5 * time.Minute).UTC()}
	configuration, err := cryptoadapter.FrostConfiguration(publics, 3)
	if err != nil {
		return err
	}
	signerIDs := []string{"p1", "p2", "p3"}[:*signerCount]
	for i, node := range nodes[:*signerCount] {
		_, err := node.(*realTCPNode).call(api.Request{Operation: "real-sign-configure",
			RealSignConfig: &api.RealSignConfig{Results: publics, Threshold: 3, Policy: policy}})
		if err != nil {
			return fmt.Errorf("configure signer %s: %w", signerIDs[i], err)
		}
	}
	commitments := make(frost.CommitmentList, 0, *signerCount)
	for i, node := range nodes[:*signerCount] {
		response, err := node.(*realTCPNode).call(api.Request{Operation: "real-sign-commit", PaymentRequest: &request})
		if err != nil {
			return fmt.Errorf("commit %s: %w", signerIDs[i], err)
		}
		commitment := new(frost.Commitment)
		if err := commitment.Decode(response.SignCommitment); err != nil {
			return err
		}
		commitments = append(commitments, commitment)
	}
	commitments.Sort()
	if *tamperAfterCommit {
		request.AmountMinor++
	}
	message, err := request.Canonical()
	if err != nil {
		return err
	}
	shares := make([]*frost.SignatureShare, 0, *signerCount)
	for i, node := range nodes[:*signerCount] {
		response, err := node.(*realTCPNode).call(api.Request{Operation: "real-sign-share",
			RealSignShare: &api.RealSignShare{Request: request, Commitments: commitments.Encode()}})
		if err != nil {
			return fmt.Errorf("sign %s: %w", signerIDs[i], err)
		}
		share := new(frost.SignatureShare)
		if err := share.Decode(response.SignShare); err != nil {
			return err
		}
		shares = append(shares, share)
	}
	signature, err := configuration.AggregateSignatures(message, shares, commitments, true)
	if err != nil {
		return err
	}
	if err := frost.VerifySignature(frost.Ed25519, message, signature, configuration.VerificationKey); err != nil {
		return err
	}
	authorization := agentpayment.Authorization{Request: request, Signature: signature.Encode()[1:],
		GroupPublic: publics[0].GroupPublic, SignerIDs: signerIDs, AuthorizedAt: time.Now().UTC()}
	gateway, err := agentpayment.NewSandboxGateway(policy, publics[0].GroupPublic)
	if err != nil {
		return err
	}
	receipt, err := gateway.Pay(authorization, time.Now())
	if err != nil {
		return err
	}
	if receipt.Status != "simulated_paid" {
		return errors.New("sandbox payment did not complete")
	}
	keyHash := sha256.Sum256(publics[0].GroupPublic)
	result := agentPaymentResult{Revision: buildRevision(), Mode: "sandbox_only",
		DKGTopology: "direct_authenticated_peers", Threshold: 3, SignerIDs: signerIDs,
		GroupKeySHA256: hex.EncodeToString(keyHash[:]), Authorization: authorization,
		Receipt: receipt, DurationMS: time.Since(started).Milliseconds()}
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
