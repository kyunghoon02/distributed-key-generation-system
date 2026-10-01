# Implementation Plan

Work one milestone at a time. A milestone is complete only when its exit criteria pass and the evidence is recorded. Do not describe planned behavior as implemented in the README, portfolio, or resume.

## M0 — Baseline Runtime

- [x] Initialize Go module.
- [x] Define participant, session, and message types.
- [x] Define transport abstraction.
- [x] Implement explicit protocol state machine.
- [x] Implement deterministic mock crypto adapter.
- [x] Implement participant process and controller/CLI.
- [x] Add deterministic normal integration test with multiple participant processes.
- [x] Pass M0 exit criteria: participants communicate, normal ceremony reaches terminal success, transitions are explicit and testable.
- [x] Record executed test evidence and update README status.

M0 verification on 2026-10-01 (Go 1.24.4, darwin/arm64): `go test ./...` and `go test -race ./...` passed. `go run ./cmd/dkgctl run --participants 4 --threshold 3` returned `finalized: true`; all four participants reported `received: 4`, `expected: 4`, `threshold: 3`, and `INIT → DEAL → SHARE_EXCHANGE → VERIFY → FINALIZE`. This verifies the normal mock ceremony only; failure handling and cryptographic security remain outside M0.

## M1 — Message Semantics

- [x] Add stable logical MessageID.
- [x] Validate session, epoch, round, phase, sender, recipient, and message identity before mutation.
- [x] Deduplicate logical messages and make application idempotent.
- [x] Reject stale messages without state mutation.
- [x] Test duplicate delivery, stale delivery, and zero duplicate transitions.

M1 verification on 2026-10-01 (Go 1.24.4, darwin/arm64): `go test ./...` and `go test -race ./...` passed. Protocol tests cover repeated SHARE delivery before and after phase changes, stale ceremony coordinates, invalid identity and payload, and unchanged state on rejection. A local TCP test covers duplicate and stale delivery through the participant server. These tests use mock contributions; deterministic fault schedules and experiment measurements remain M3–M4 work.

## M2 — Crash Recovery

- [x] Add WAL or equivalent durable state.
- [x] Replay durable events on process restart.
- [x] Resume the same ceremony from SHARE.
- [x] Reproduce crash during SHARE and record recovery duration.
- [x] Verify replay consistency and no duplicate state mutation.

M2 verification on 2026-10-01 (Go 1.24.4, darwin/arm64): `go test ./...` and `go test -race ./...` passed. `TestCrashRecoveryDuringShare` killed one of three participant processes after it had accepted one peer SHARE (2/3 contributions), then restarted it with the same WAL. Replayed status and transitions matched the pre-crash state; retrying `begin` and the same SHARE added no WAL record or state transition. All three participants reached `FINALIZE`, and a second restart restored the terminal state. One local run measured 22.534375 ms from process restart to the first recovered status response. This is a single observation, not a latency guarantee or a complete E3 experiment record.

## M3 — Fault Injection

- [x] Add deterministic delay, drop, duplicate, crash/restart, and partition policies.
- [x] Make E1–E6 repeatable from tests or an experiment runner.
- [x] Record terminal state and phase deadline behavior.
- [x] Confirm threshold-deficient partition never finalizes.

M3 verification on 2026-10-01: `TestDeterministicExperimentScenarios` passed for E0–E6. The local runner schedules message duplication, old-round delivery, process kill/restart, message loss, a 2:2 partition, and delayed SHARE messages. An incomplete ceremony enters durable `TIMED_OUT`; late messages cannot revive it. In E5, every participant had only 2/4 contributions and none finalized. These are mock-runtime outcomes, not a real DKG liveness claim.

## M4 — Observability and Evidence

- [x] Add structured logs.
- [x] Add low-cardinality Prometheus metrics.
- [x] Capture experiment results as JSON and human-readable output.
- [x] Tie each portfolio claim to a passing test or recorded experiment.
- [x] Put measured results in README only after execution.

M4 verification on 2026-10-01: `go test ./...` and `go test -race ./...` passed, including tests for the participant `/metrics` endpoint, bounded label values, structured JSON logs, and both experiment output formats. Seven JSON result files under `results/mock/2026-10-01/` were generated from code revision `7737678e9d43295dad67c9318dbdb0605a24031f` with a 300 ms SHARE deadline. Their timings are one local run each; see `docs/experiments.md` for definitions and limits.

## M5 — Real DKG Integration

- [x] Research maintained Go-compatible DKG libraries.
- [x] Document protocol, maintenance status, real-world usage, assumptions, and license for candidates.
- [x] Select an implementation with a documented rationale; no independent audit of this integration is claimed.
- [x] Integrate it behind a real multi-round Crypto Adapter and local TCP participant processes.
- [x] Re-run E0–E6 process fault schedules with real cryptographic packets; E3 follows a fresh-session recovery policy.
- [x] Document protocol-specific assumptions and limit claims to observed local evidence.

M5 verification on 2026-10-01 (Go 1.25.0, darwin/arm64): `go test ./...`, `go test -race ./...`, and `go vet ./...` passed. Four local participant processes ran drand/kyber `v1.3.2` Pedersen DKG with Ed25519, signed packets, encrypted deal shares, and a 3-of-4 threshold. E0–E2 finalized at all four participants; E4 finalized at the three responsive participants with one group public key; E5's 2:2 split aborted with no finalization; E6 finalized at three and rejected the held packet after terminal state. E3 crashed p1 after one peer deal, aborted the interrupted ceremony, and completed a new one with fresh identities and nonce. These results are recorded under `results/real/2026-10-01/` from revision `f5e38932c9d40a116422d399c3570c462df917bb`. See [library selection](dkg-library-selection.md) and [experiment evidence](experiments.md).

Real same-session DKG state restoration, authenticated remote RPC, key custody, and a security assessment remain outside this local M5 integration. The mock M2 WAL must not be presented as recovery for Kyber's private in-memory state.

## M6 — Real Ceremony Supervision and Local Channel Authentication

- [x] Run a normal real ceremony through a process supervisor, not an experiment-only E3 branch.
- [x] Stop all local processes after an RPC/process failure and retry with fresh participant keys and nonce.
- [x] Record started, aborted, and finalized controller decisions in a synced journal without private shares; reject a signed packet from the aborted session in E3.
- [x] Require mutual TLS on real participant RPC; verify the expected server name and controller certificate.
- [x] Expose bounded Prometheus metrics from real participant processes.
- [x] Verify in an isolated adapter test that three finalized shares reconstruct a key that signs for the DKG group public key and that two shares cannot reconstruct it.

M6 remains a local runtime milestone. Test-only secret reconstruction does not implement distributed threshold signing. The controller journal does not restore Kyber private state, and an abrupt controller crash may leave old local child processes running until cleaned up. Multi-host deployment, certificate lifecycle, key custody, distributed threshold signing, and security review remain future work.

M6 verification on 2026-10-01 (Go 1.25.0, darwin/arm64): `go test ./...`, `go test -race ./...`, `go vet ./...`, `go mod verify`, and three consecutive `go test` runs of the process integration and crypto-adapter packages passed. The TLS test accepts the controller certificate and rejects an anonymous client and a mismatched server name. The process test covers E3's abort, fresh-process retry, fresh nonce, and old signed deal rejection; a separate test recovers an interrupted controller journal. Revision `b5b7da0f41541461e5a131fe5f604ae84bb85171` was then built cleanly and used for the [M6 local result files](experiments.md#m6-mutual-tls-rerun). These are local functional checks, not a production security assessment.

## M7 — Direct Peer Packet Exchange

- [x] Add a local 4-node direct TCP/TLS packet path with certificate identity bound to the configured roster and signed sender.
- [x] Keep the controller on setup, start, status, and public-result RPC for the direct path.
- [x] Verify a complaint-free E0 ceremony finalizes all four nodes with one group public key.
- [ ] Implement complaint/justification completion and participant-loss behavior on the direct path.
- [ ] Add peer-mode abort, fencing, and fresh-session recovery to the supervised normal command.
- [ ] Validate multi-host addresses, certificate provisioning, and network faults.

`real-p2p-run` is a local proof of direct packet exchange. It waits for all peers in each round, retries sends within a bounded deadline, and aborts on a timeout or justification requirement. The E0–E6 experiments and `real-ceremony` still use the controller relay; direct path fault tolerance remains unverified.

M7 local verification on 2026-10-01: `go test ./...`, `go test -race ./...`, `go vet ./...`, and `git diff --check` passed. The peer process integration test passed three consecutive runs; each starts four separate processes and checks four finalizations and group-key agreement. A TLS authorization test rejects controller packet/phase RPC and peer access to control RPC, and checks that the certificate identity cannot impersonate a different packet sender. One direct CLI run also finalized all four local participants with the same group public key. These checks do not establish behavior under packet loss, complaints, process crashes, or multi-host networking.

## M8 — Agent Payment Authorization Sandbox

- [x] Define a typed payment request, locally checked spending policy, canonical signing bytes, and sandbox receipt.
- [x] Import each Kyber DKG secret share into FROST inside its participant process after checking all public shares against the DKG commitments.
- [x] Produce a 3-of-4 Ed25519-verifiable authorization without reconstructing the private key in the controller.
- [x] Reject invalid merchant, per-payment over-limit amount, changed request after commitments, and fewer than three signers in local process tests.
- [x] Verify the authorization at an in-memory sandbox gateway and handle exact replay idempotently.
- [ ] Make key shares, signing nonces, budget reservations, and receipts durable across process restart.
- [ ] Separate policy-owner authority from the local controller and validate multi-host trust boundaries.
- [ ] Integrate a sandbox payment rail and its settlement/idempotency semantics.

The [use-case document](agent-payment-use-case.md) gives the data flow, trust assumptions, and evidence boundary. This milestone authorizes no real payments.

M8 local verification on 2026-10-01 (Go 1.26.3, darwin/arm64): `go test ./...`, `go test -race ./...`, `go test -count=1 ./tests`, `go vet ./...`, `go mod verify`, and `git diff --check` passed. The process test builds the CLI, completes a direct 4-process DKG, verifies the resulting threshold signature with Go's standard Ed25519 verifier, checks the simulated receipt with p4 online and offline after DKG, and rejects invalid merchant, over-limit amount, two signers, and changed request contents. This is local functional evidence only.
