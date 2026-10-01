# Distributed Key Generation System

A fault-tolerant distributed key generation runtime for studying how DKG behaves under real network and process failures.

This project focuses on the distributed runtime around a DKG ceremony: explicit participant state, message delivery semantics, durable recovery, and reproducible failure experiments. Cryptographic operations are isolated behind an adapter so runtime behavior can be developed independently.

> **Status:** M0 normal flow, M1 mock SHARE message semantics, and M2 local process crash recovery passed their tests on 2026-10-01 with Go 1.24.4. M3–M5 remain planned, and full experiment result records remain pending. The implementation uses deterministic mock cryptography and makes no cryptographic security claim.

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

The controller coordinates separate local participant processes over a transport interface, and participants execute an explicit state machine using a deterministic mock crypto adapter. A per-participant WAL now preserves accepted state changes across process restart. The controller does not yet restart failed processes automatically; fault injection and metrics are later targets.

## Failure matrix

These are target outcomes. M1 tests cover duplicate and stale mock SHARE delivery, and an M2 test covers local crash recovery. Full E0–E6 experiment records remain pending.

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
| M3 — Fault Injection | Deterministic delay, drop, duplicate, crash/restart, and partition schedules | Planned |
| M4 — Observability and Evidence | Structured logs, Prometheus metrics, machine-readable experiment results, regression tests | Planned |
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
```

`dkgctl participant --id p1 --listen 127.0.0.1:9001 --state-file ./state/p1.wal` keeps one participant's state across process restarts when launched again with the same ID and state file. The normal `run` command uses temporary state files and removes them when it exits.

Milestone-specific implementation and exit criteria are tracked in [`docs/implementation-plan.md`](docs/implementation-plan.md). Experiment procedures and result fields are in [`docs/experiments.md`](docs/experiments.md). No benchmark or experiment result will be reported here until it has been run and captured.
