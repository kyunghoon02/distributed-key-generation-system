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

- [ ] Add deterministic delay, drop, duplicate, crash/restart, and partition policies.
- [ ] Make E1–E6 repeatable from tests or an experiment runner.
- [ ] Record terminal state and phase deadline behavior.
- [ ] Confirm threshold-deficient partition never finalizes.

## M4 — Observability and Evidence

- [ ] Add structured logs.
- [ ] Add low-cardinality Prometheus metrics.
- [ ] Capture experiment results as JSON and human-readable output.
- [ ] Tie each portfolio claim to a passing test or recorded experiment.
- [ ] Put measured results in README only after execution.

## M5 — Real DKG Integration

- [ ] Research maintained Go-compatible DKG libraries.
- [ ] Document protocol, maintenance status, real-world usage, assumptions, and license for candidates.
- [ ] Select a maintained/reviewed implementation with a documented rationale.
- [ ] Integrate it behind the Crypto Adapter.
- [ ] Re-run runtime fault tests with real cryptographic payloads.
- [ ] Document protocol-specific assumptions and limit security claims to supported evidence.
