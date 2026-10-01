# Architecture

## Goal

The runtime coordinates a fixed participant set through a DKG ceremony while preserving protocol state across unreliable message delivery and process failures. The architecture separates distributed runtime concerns from cryptographic operations.

## Target components

```text
dkgctl → participant processes → transport → fault injection

participant:
network transport → message validation → protocol state machine
                 → crypto adapter → durable state → metrics
```

### Controller (`dkgctl`)

Creates or drives a ceremony, starts/contacts participant processes, and collects terminal status. It must not be the source of cryptographic truth. Early milestones may use it to coordinate deterministic test runs.

### Participant process

Owns one participant identity and its ceremony state. Each participant advances through explicit phases rather than running the ceremony as one opaque function.

### Transport

Provides message delivery between participant processes. Its interface is independent of the protocol and will be wrapped by deterministic fault policies in M3.

### Message validation

Checks the ceremony/session, epoch, round, phase, sender, recipient, and logical message identity before protocol state is mutated. Validation and deduplication are introduced in M1.

### Protocol state machine

Initial phases are `INIT → DEAL → SHARE_EXCHANGE → VERIFY → FINALIZE`, with `COMPLAINT` / `CONFIRM` added when the selected protocol requires them. Transitions are monotonic and testable.

### Crypto adapter

Defines the boundary for protocol-specific cryptographic work. Early milestones use deterministic mock behavior. The mock is not a cryptographic implementation. M5 selects a maintained, reviewed Go-compatible library only after documenting protocol, maintenance, usage, assumptions, and license.

### Durable state

M2 uses one append-only JSON-line WAL per participant. Accepted `begin`, SHARE, and `finalize` events are synced before the participant acknowledges them. A restarted process replays the same file before serving requests; replay restores applied message IDs as well as protocol state. An incomplete trailing record is discarded, while a malformed complete record blocks startup. The state file assumes one active writer and storage that survives process restart; node loss and automated process restart are outside M2.

### Observability

M4 adds structured logs, low-cardinality metrics, and JSON experiment records. Raw session IDs belong in logs, not Prometheus labels.

## M0 boundary

M0 proves only that multiple local participant processes can complete a deterministic mock ceremony through an explicit state machine. It does not prove reliability under faults, durable recovery, Byzantine tolerance, or cryptographic security.
