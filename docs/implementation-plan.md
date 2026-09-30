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
- [ ] Pass M0 exit criteria: participants communicate, normal ceremony reaches terminal success, transitions are explicit and testable.
- [ ] Record executed test evidence and update README status.

## M1 — Message Semantics

- [ ] Add stable logical MessageID.
- [ ] Validate session, epoch, round, phase, sender, recipient, and message identity before mutation.
- [ ] Deduplicate logical messages and make application idempotent.
- [ ] Reject stale messages without state mutation.
- [ ] Test duplicate delivery, stale delivery, and zero duplicate transitions.

## M2 — Crash Recovery

- [ ] Add WAL or equivalent durable state.
- [ ] Replay durable events on process restart.
- [ ] Resume the same ceremony from SHARE.
- [ ] Reproduce crash during SHARE and record recovery duration.
- [ ] Verify replay consistency and no duplicate state mutation.

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
