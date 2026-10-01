# Distributed Key Generation System

A fault-tolerant distributed key generation runtime for studying how DKG behaves under real network and process failures.

This project studies the distributed runtime around a DKG ceremony: explicit participant state, message delivery semantics, durable recovery, and reproducible failure experiments. Its application demo is an **agent payment authorization gateway**: four separate local processes generate one key and three of them sign a policy-checked payment request before a sandbox gateway accepts it. No real payment is executed.

> **Status:** M0–M4 mock-runtime tests, M5 real-DKG fault experiments, M6 local ceremony supervision, M7 local direct packet exchange, and an M8 sandbox agent-payment signing flow passed on 2026-10-01. Real DKG uses drand/kyber `v1.3.2`; the sandbox signing flow imports each participant's DKG share locally into FROST and produces an Ed25519-verifiable 3-of-4 authorization. The direct path currently covers a complaint-free E0 ceremony on loopback. Real shares, payment policy reservations, and sandbox receipts are memory-only. No payment rail, multi-host operation, or production security has been established.

## Problem

Protocol descriptions often assume authenticated, reliable channels. Real processes and networks introduce duplicate, delayed, stale, reordered, or lost messages; restarts after partial progress; partial availability; partitions; and timeout decisions. A distributed DKG runtime must preserve protocol safety across those conditions and make progress only when the selected protocol's rules allow it.

For an autonomous payment agent, the practical question is who controls spending authority when the agent requests a purchase. The sandbox flow requires a canonical request with a merchant, amount, budget, key version, request ID, and expiry. Each signing node checks a locally provisioned policy before participating. The gateway verifies one threshold signature and records an idempotent simulated payment. See [agent payment use case](docs/agent-payment-use-case.md) for the trust boundary and remaining work.

## Architecture

Current local experiment path:

```text
dkgctl
  ├─ deterministic packet delivery / fault schedules
  └─ TCP or mutual TLS RPC → participant processes
```

Participant internals differ by path:

```text
mock: TCP → message validation → explicit state machine → mock adapter → WAL → metrics
real: mutual TLS → signed packet validation → Kyber DKG → in-memory share → metrics
```

The controller coordinates separate local participant processes. `real-run` and `real-ceremony` relay DKG packets through the controller; `real-p2p-run` configures the roster and then nodes send signed deal and response packets directly to one another over mutual TLS. The mock path has an explicit state machine, deterministic mock crypto, and a per-participant WAL for same-session process recovery. The real path wraps drand/kyber's rounds behind a crypto adapter; its secret state stays in process memory. The local process runner creates a short-lived certificate authority and controller and participant certificates. `agent-payment-demo` adds local FROST signing and sandbox verification after direct DKG. Fault schedules still run on the controller relay path. Mock and real participants expose separate Prometheus metrics and structured request logs. The [proposed final architecture](docs/architecture.md#proposed-final-architecture) extends these local flows with durable custody, multi-host operation, independent policy authority, and payment integration.

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
| M6 — Real Ceremony Supervision | Retry failed local process ceremonies with fresh identities and nonce; record decisions; authenticate RPC; verify share threshold in a test | Verified locally; multi-host deployment and distributed signing remain future work |
| M7 — Direct Peer Packet Exchange | Four local nodes send DKG packets to one another over mutual TLS TCP | E0 verified; direct-path fault and recovery behavior remains open |
| M8 — Agent Payment Authorization Sandbox | Use finalized DKG shares for 3-of-4 FROST signing of a typed payment request; verify and record a simulated payment | Verified locally; policy and custody are not durable, and no real payment integration exists |

## Non-goals

- Building a blockchain
- Implementing full BFT consensus
- Implementing elliptic-curve primitives from scratch
- Delivering a production validator implementation
- Adding Kubernetes deployment solely to increase apparent complexity
- Claiming production cryptographic security from a local integration test

## Development

Go 1.26.3 or newer is required. The mock runtime uses deterministic contributions for orchestration tests. The real DKG path uses random participant keys and drand/kyber protocol packets; local success does not establish production security.

```sh
go test ./...
go run ./cmd/dkgctl run --participants 4 --threshold 3
go run ./cmd/dkgctl experiment --scenario E5 --format text --output results/E5.json
go run ./cmd/dkgctl real-run --scenario E0
go run ./cmd/dkgctl real-run --scenario E4
go run ./cmd/dkgctl real-p2p-run
go run ./cmd/dkgctl agent-payment-demo
go run ./cmd/dkgctl agent-payment-demo --offline-p4
go run ./cmd/dkgctl real-ceremony --journal .dkgctl/real-ceremony.jsonl
```

`dkgctl participant --id p1 --listen 127.0.0.1:9001 --state-file ./state/p1.wal` keeps one participant's state across process restarts when launched again with the same ID and state file. The normal `run` command uses temporary state files and removes them when it exits.

`real-ceremony` runs a normal 3-of-4 ceremony. On a detected participant RPC or process failure it stops all four local processes, records an abort, and retries with new identities and nonce (at most two attempts by default). Its controller journal contains only nonce hashes and decisions; Kyber private state is never replayed. The E3 fault experiment uses the same supervisor and verifies that an old signed deal is rejected by the fresh session. An interrupted controller run is marked aborted on its next start; orphaned child processes from an abrupt controller crash are not currently reaped across controller restarts.

`real-p2p-run` starts four local participants, gives each the same roster and peer addresses, and starts each node's own DKG loop. Nodes exchange packets directly using TCP/TLS + JSON; the controller only configures, starts, and reads public results. The peer mode accepts packet RPC only from a roster member whose client certificate identity matches the signed packet sender. It waits for all three peers in each round and currently aborts if the ceremony needs justifications. This command has no crash recovery or fault schedule yet.

`agent-payment-demo` uses the same direct local DKG, configures a payment policy on three signers, obtains one-time FROST commitments and signature shares, aggregates a 3-of-4 Ed25519 signature, and submits the authorization to an in-memory sandbox gateway. The gateway verifies the exact typed request and returns a simulated receipt. `--offline-p4` stops one node after DKG; `--merchant forbidden.example`, `--amount-minor 6000`, `--signer-count 2`, and `--tamper-after-commit` demonstrate rejected requests. The demo does not transfer funds or persist budget state, key shares, or signing nonces.

Add `--metrics-listen 127.0.0.1:9002` to a mock or real participant command to expose `/metrics`; participant requests are logged as JSON to stderr. Real participant RPC requires `--tls-cert`, `--tls-key`, and `--tls-client-ca`. Real metrics count bounded RPC operations, results, packet outcomes, and phase; they carry no session ID or participant-provided label. The supervisor generates local certificates automatically. Externally supplied certificates and multi-host networking have not been exercised end to end.

The seven [recorded mock experiments](docs/experiments.md) used a 300 ms SHARE deadline. E0–E3 completed; E4–E6 timed out. In the [real DKG results](docs/experiments.md), E0–E2 finalized at all four participants; E4 and E6 finalized at three; E5 finalized at none. E3 aborted the interrupted session and completed a new one. The M6 rerun used mutual TLS and the common E3 supervisor. Each timing is a single local observation, not a performance guarantee.

Milestone criteria are tracked in [`docs/implementation-plan.md`](docs/implementation-plan.md). [Current and proposed architecture](docs/architecture.md) separates the running code from the target signing system. The [library selection](docs/dkg-library-selection.md) records protocol assumptions and limitations; [`docs/experiments.md`](docs/experiments.md) links the result files.
