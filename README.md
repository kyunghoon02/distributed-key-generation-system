# Distributed Key Generation System

A fault-tolerant distributed key generation runtime for studying how DKG behaves under real network and process failures.

This project focuses on the distributed runtime around a DKG ceremony: explicit participant state, message delivery semantics, durable recovery, and reproducible failure experiments. Cryptographic operations are isolated behind an adapter so runtime behavior can be developed independently.

> **Status:** M0–M4 mock-runtime tests and M5 local real-DKG experiments E0–E6 passed on 2026-10-01. Real DKG uses drand/kyber `v1.3.2` in four separate local processes. The real path has no same-session WAL recovery, remote transport security, or production security claim.

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

The controller coordinates separate local participant processes over TCP. The mock path has an explicit state machine, deterministic mock crypto, and a per-participant WAL for same-session process recovery. The real path wraps drand/kyber's deal, response, and justification rounds behind a crypto adapter. Its secret state stays in process memory. Both experiment runners inject local delivery and process faults; the mock participant exposes Prometheus metrics, and both paths emit structured request logs.

## Failure matrix

These are target outcomes. The [experiment records](docs/experiments.md) keep mock and real DKG results separate.

| Failure | Target |
|---|---|
| Duplicate SHARE | Apply the state transition once |
| Stale message | Reject without mutating state |
| Crash during SHARE | Mock: replay WAL and resume; real DKG: abort and start a fresh ceremony |
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
| M5 — Real DKG Integration | Select drand/kyber; run signed packets with encrypted deal shares through four local processes and E0–E6 faults | Verified locally, with documented deployment and recovery limits |

## Non-goals

- Building a blockchain
- Implementing full BFT consensus
- Implementing elliptic-curve primitives from scratch
- Delivering a production validator implementation
- Adding Kubernetes deployment solely to increase apparent complexity
- Claiming production cryptographic security from a local integration test

## Development

Go 1.25 or newer is required. The mock runtime uses deterministic contributions for orchestration tests. The real DKG path uses random participant keys and drand/kyber protocol packets; local success does not establish production security.

```sh
go test ./...
go run ./cmd/dkgctl run --participants 4 --threshold 3
go run ./cmd/dkgctl experiment --scenario E5 --format text --output results/E5.json
go run ./cmd/dkgctl real-run --scenario E0
go run ./cmd/dkgctl real-run --scenario E4
```

`dkgctl participant --id p1 --listen 127.0.0.1:9001 --state-file ./state/p1.wal` keeps one participant's state across process restarts when launched again with the same ID and state file. The normal `run` command uses temporary state files and removes them when it exits.

Add `--metrics-listen 127.0.0.1:9002` to a participant command to expose `/metrics`; participant requests are logged as JSON to stderr. Metrics use bounded operation, result, and SHARE outcome labels. Session IDs appear in logs and result files, not metric labels.

The seven [recorded mock experiments](docs/experiments.md) used a 300 ms SHARE deadline. E0–E3 completed; E4–E6 timed out. In the [real DKG results](docs/experiments.md), E0–E2 finalized at all four participants; E4 and E6 finalized at three; E5 finalized at none. E3 aborted the interrupted session and completed a new one. Each timing is a single local observation, not a performance guarantee.

Milestone criteria are tracked in [`docs/implementation-plan.md`](docs/implementation-plan.md). The [library selection](docs/dkg-library-selection.md) records protocol assumptions and limitations; [`docs/experiments.md`](docs/experiments.md) links the result files.
