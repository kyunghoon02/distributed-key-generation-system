# Architecture

## Goal

The runtime coordinates a fixed participant set through a DKG ceremony while preserving protocol state across unreliable message delivery and process failures. The architecture separates distributed runtime concerns from cryptographic operations.

## Components

```text
dkgctl → participant processes → transport → fault injection

mock participant:
TCP → message validation → explicit state machine → mock adapter → WAL → metrics

real participant:
loopback TCP → session and signature validation → Kyber DKG adapter
                                              → in-memory secret state
```

### Controller (`dkgctl`)

Creates or drives a ceremony, starts/contacts participant processes, and collects terminal status. It must not be the source of cryptographic truth. Early milestones may use it to coordinate deterministic test runs.

### Participant process

Owns one participant identity and its ceremony state. The mock path advances through its explicit state machine; the real path drives Kyber's deal, response, and justification rounds in a separate participant process. The real controller sees signed packets and public results, not private shares.

### Transport

Provides local TCP request and packet delivery. The controller applies deterministic fault schedules before forwarding packets. Real participant listeners accept loopback addresses only; the RPC channel does not authenticate clients.

### Message validation

The mock path checks session, epoch, round, phase, sender, recipient, and logical message identity. The real adapter checks a fresh session nonce, fixed sender/index mapping, signed packet, and duplicate identity before passing the packet to Kyber.

### Protocol state machine

Mock phases are `INIT → DEAL → SHARE_EXCHANGE → VERIFY → FINALIZE` or terminal `TIMED_OUT`. The real adapter progresses through `INIT → DEAL → COLLECT_DEALS → COLLECT_RESPONSES → COLLECT_JUSTIFICATIONS → FINALIZE` or terminal `ABORTED`/`TIMED_OUT`. The Kyber library decides qualification and uses signed responses and justifications when needed.

### Crypto adapter

The mock adapter produces deterministic test contributions. The [Kyber adapter](../internal/cryptoadapter/kyber.go) uses drand/kyber `v1.3.2` for real Pedersen DKG payloads. The two adapters have different round interfaces because one mock SHARE cannot represent a real multi-round protocol. Selection evidence is in [the library decision](dkg-library-selection.md).

### Durable state

The mock M2 path uses one append-only JSON-line WAL per participant. Accepted `begin`, SHARE, and `finalize` events are synced before acknowledgement. A restarted mock process replays the same file before serving requests; replay restores applied message IDs and protocol state. The real Kyber engine retains random polynomial state in memory and has no safe same-session WAL replay. Real E3 aborts the old session and restarts all participants with fresh identities and nonce.

### Observability

The mock path emits structured JSON request logs, Prometheus request and SHARE outcome counters, a request-duration histogram, and a current-phase gauge. The real path emits JSON request logs without packet payloads; it does not yet expose Prometheus metrics. Both controllers write E0–E6 JSON result files. Timing fields are controller observations, not cryptographic phase benchmarks.

## M0 boundary

M0 proves only that multiple local participant processes can complete a deterministic mock ceremony through an explicit state machine. It does not prove reliability under faults, durable recovery, Byzantine tolerance, or cryptographic security.
