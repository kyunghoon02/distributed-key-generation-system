# Agent Payment Authorization Sandbox

## User story

An autonomous agent may propose a payment to a paid API or merchant. It cannot authorize spending alone. Four independently operated signer services create one key with DKG. A request is authorized only when three services independently check the same policy and produce one threshold signature. A payment gateway verifies the authorization before it executes a payment.

The repository currently runs this story only with four loopback processes and an in-memory **sandbox gateway**. The command is `go run ./cmd/dkgctl agent-payment-demo`. It records `simulated_paid` and never contacts a card processor, wallet, x402 facilitator, or live payment network. The contract is inspired by the signed, constraint-bound authorization model in the [AP2 specification](https://github.com/google-agentic-commerce/AP2/blob/main/docs/ap2/specification.md); this is not an AP2 implementation.

## Current flow

```text
controller: configure 3-of-4 DKG roster, nonce, key ID/version
       ↓
p1 ↔ p2 ↔ p3 ↔ p4: signed DKG packets over mutual TLS TCP
       ↓
each process: private DKG share retained in memory
       ↓
controller: distribute public DKG results and sandbox payment policy
       ↓
p1, p2, p3: validate typed request and reserve local budget
       ↓
p1, p2, p3: one-time FROST commitments → signature shares
       ↓
controller: verify shares, aggregate and verify Ed25519 signature
       ↓
sandbox gateway: verify signature and policy → idempotent simulated receipt
```

The controller sees public DKG results, payment requests, commitments, signature shares, and the final signature. It does not receive private DKG shares. Each signer verifies that the published shares match its DKG public commitments before importing its own secret share into FROST inside the same process. The [FROST specification](https://www.rfc-editor.org/rfc/rfc9591.html) defines the two-round threshold signing protocol and Ed25519-compatible signatures; DKG is outside that specification.

## Application contract

`Request` identifies the request, agent, merchant, budget, signing key and version, currency, integer amount in minor units, and expiry. A versioned canonical byte encoding is the exact input to FROST. The controller cannot change the amount, merchant, or request ID after collecting commitments without signer rejection. Every signer checks its configured policy: identity and key match, allowed merchant, per-payment limit, remaining total limit, and expiry.

`Authorization` contains the typed request, standard 64-byte Ed25519 signature, group public key, and observed signer IDs. The signer-ID list is audit metadata; the signature itself proves threshold participation under the FROST configuration. `SandboxGateway.Pay` verifies the signature and policy, then records one receipt per request ID. Repeating the same signed request returns the same receipt. A modified request fails signature verification.

DKG packets are separate from payment messages. `internal/protocol.Message` remains the mock-runtime SHARE envelope. The real DKG packet identifies its nonce/session, sender, and deal/response/justification kind. `internal/agentpayment.Request` is the application-level signing input.

## Evidence and limits

The process integration test builds the CLI and checks a valid payment, one signer unavailable after DKG, invalid merchant, amount over the per-payment limit, two available signers, and a changed amount after commitments. Unit tests check policy and gateway signature/replay behavior. These are local functional tests.

The current policy comes from the local controller and is trusted at provisioning time. Separate policy-owner authorization and independent administrative domains have not been implemented. Budget reservations, receipts, private shares, and FROST nonce state are memory-only. A failed signing attempt consumes the local reservation to avoid an unsafe retry, but it is not a durable accounting system. Local signer reservations **do not establish a global total budget across changing 3-of-4 signer sets**; the sandbox gateway is the authoritative total-budget ledger. The DKG peer path waits for all four peers and does not yet handle complaints, process recovery, or multi-host operation. The signing coordinator can stop progress; it cannot produce a valid signature with fewer than three uncompromised shares. A real payment executor must enforce this authorization and its own idempotency and settlement rules before any funds move.

## Next boundaries

1. Provision policies under a separate owner identity; bind policy version and key epoch to the verified key registry.
2. Persist encrypted private shares, signing nonce state, budget reservations, and receipts with explicit restart/fencing behavior.
3. Complete direct DKG fault handling and peer-mode recovery; test signer outage and network partitions across hosts.
4. Integrate one sandbox payment rail behind a verifier and map its receipt and reversal semantics. Add real money only after a security review and operational controls.
