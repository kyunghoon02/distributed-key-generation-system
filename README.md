# Distributed Key Generation System

A fault-tolerant distributed key generation runtime for studying how DKG behaves under real network and process failures.

This project focuses on the distributed runtime around a DKG ceremony: explicit participant state, message delivery semantics, durable recovery, and reproducible failure experiments. Cryptographic operations are isolated behind an adapter so runtime behavior can be developed independently.

> **Status:** M0–M4 have passing local tests and recorded mock-runtime experiments as of 2026-10-01. M5 real DKG integration remains planned. The implementation uses deterministic mock cryptography and makes no cryptographic security claim.

## Problem

Protocol descriptions often assume authenticated, reliable channels. Real processes and networks introduce duplicate, delayed, stale, reordered, or lost messages; restarts after partial progress; partial availability; partitions; and timeout decisions. A distributed DKG runtime must preserve protocol safety across those conditions and make progress only when the selected protocol's rules allow it.

## Architecture

Target control and data path:

```text
dkgctl
  └─ participant processes
       └─ transport
            └─ deterministic fault injection
```

Participant internals:

```text
Network Transport → Message Validation → Protocol State Machine
                  → Crypto Adapter → Durable State → Metrics
```

The controller coordinates separate local participant processes over a transport interface, and participants execute an explicit state machine using a deterministic mock crypto adapter. A per-participant WAL preserves accepted state changes across process restart. The experiment runner applies deterministic local delivery and process fault schedules; participants expose structured logs and Prometheus metrics. The normal `run` command does not restart failed processes automatically.

## Failure matrix

These are target outcomes. Local mock-runtime results for E0–E6 are recorded in [`docs/experiments.md`](docs/experiments.md); real DKG behavior remains unverified.

| Failure | Target |
|---|---|
| Duplicate SHARE | Apply the state transition once |
| Stale message | Reject without mutating state |
| Crash during SHARE | Recover durable state and resume |
| One unavailable participant | Continue only when protocol and threshold rules permit |
| 2:2 partition, threshold 3 | Do not finalize |
| Slow participant | Make phase deadline behavior observable |

## Milestones

| Milestone | Scope | Status |
|---|---|---|
| M0 — Baseline Runtime | Go module, participant processes, controller, message model, transport abstraction, explicit state machine, deterministic normal run | Verified for the normal flow |
| M1 — Message Semantics | Stable message IDs, session/epoch/round/phase validation, deduplication, idempotent application, stale rejection | Verified for mock SHARE delivery |
| M2 — Crash Recovery | WAL or equivalent, replay, process restart, resume | Verified for local mock SHARE recovery |
| M3 — Fault Injection | Deterministic delay, drop, duplicate, crash/restart, and partition schedules | Verified in local mock experiments |
| M4 — Observability and Evidence | Structured logs, Prometheus metrics, machine-readable experiment results, regression tests | Verified for local mock runtime |
| M5 — Real DKG Integration | Research and select a maintained Go-compatible DKG library; integrate behind the adapter | Planned |

## Non-goals

- Building a blockchain
- Implementing full BFT consensus
- Implementing elliptic-curve primitives from scratch
- Delivering a production validator implementation
- Adding Kubernetes deployment solely to increase apparent complexity
- Claiming cryptographic security before a real DKG integration and its assumptions are reviewed

## Development

The initial runtime uses deterministic mock cryptography. It is useful for testing orchestration and state transitions only; it does not generate cryptographically secure keys or prove a DKG protocol secure.

```sh
go test ./...
go run ./cmd/dkgctl run --participants 4 --threshold 3
go run ./cmd/dkgctl experiment --scenario E5 --format text --output results/E5.json
```

`dkgctl participant --id p1 --listen 127.0.0.1:9001 --state-file ./state/p1.wal` keeps one participant's state across process restarts when launched again with the same ID and state file. The normal `run` command uses temporary state files and removes them when it exits.

Add `--metrics-listen 127.0.0.1:9002` to a participant command to expose `/metrics`; participant requests are logged as JSON to stderr. Metrics use bounded operation, result, and SHARE outcome labels. Session IDs appear in logs and result files, not metric labels.

The seven [recorded mock experiments](docs/experiments.md) used a 300 ms SHARE deadline on 2026-10-01. E0–E3 completed; E4–E6 timed out without finalization. E3 observed 28 ms from restarting one participant to its first recovered status response in that single local run. These timings are observations, not performance guarantees.

Milestone-specific implementation and exit criteria are tracked in [`docs/implementation-plan.md`](docs/implementation-plan.md). Experiment procedures, measured fields, and result files are in [`docs/experiments.md`](docs/experiments.md).
