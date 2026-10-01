# Failure Model

## Scope

The system studies runtime safety and progress around a DKG ceremony. The selected cryptographic protocol ultimately defines its own participant and threshold rules; runtime behavior must not silently replace those rules with availability heuristics.

## Network faults

- **Delay:** a message arrives later than expected.
- **Loss:** a message is not delivered.
- **Duplication:** a logical message is delivered more than once, including after an acknowledgement is lost.
- **Reordering:** messages arrive in a different order from send order.
- **Partition:** groups of participants cannot communicate for a period of time; eventual delivery is not guaranteed while a partition remains active.

## Process faults

- **Crash-stop:** a participant stops and does not return during the ceremony.
- **Crash-recovery:** a participant stops and later restarts with access to durable local storage.

## Byzantine behavior

Byzantine behavior is not required in M0–M4. Participants are initially assumed non-Byzantine. A later extension may consider malformed, contradictory, or strategically withheld protocol messages, subject to the selected DKG protocol's threat model.

## Initial assumptions

- Membership is fixed for one ceremony.
- Participant IDs are stable within that ceremony.
- Communication is authenticated or identity-bound at the runtime level before production use; M0 local transport is a development harness, not an authentication claim.
- Durable local storage, once introduced, survives a participant process restart.
- The network may delay, drop, duplicate, and reorder messages.
- Eventual delivery is not guaranteed during an active partition.
- Early milestones assume non-Byzantine participants unless explicitly stated.
- The configured threshold and protocol-specific participation rules are authoritative; the runtime must not finalize below them.

## Safety and liveness

Safety means that an execution never accepts an invalid transition or finalizes without satisfying the configured protocol conditions. Liveness means that an execution can make progress under stated assumptions, such as enough responsive participants and eventual message delivery. A timeout can make failure explicit; it cannot make an unsafe finalization valid.

## Runtime invariants

- **S1 — Monotonic state:** a participant never transitions to an earlier phase.
- **S2 — Idempotent application:** a logical message mutates protocol state at most once.
- **S3 — Stale isolation:** an old session, epoch, round, or phase cannot mutate current state.
- **S4 — Threshold gate:** finalization requires the selected protocol's configured threshold conditions.
- **S5 — Durable replay consistency:** after M2, replay reconstructs state equivalent to the durable pre-crash state.

M0 tests the transitions needed for a normal mock run. M1 tests S2 and S3 for mock SHARE messages, M2 tests S5 across local process restart, and M3 tests terminal timeout and threshold-deficient partition behavior in the mock runtime. The protocol-specific threshold and stronger cryptographic agreement properties remain targets until their corresponding implementation and tests exist. No cryptographic security is claimed before real DKG integration.

## Liveness targets

- A normal four-participant run completes.
- When enough responsive participants remain under the selected protocol's rules, the ceremony can progress.
- A terminally impossible or threshold-deficient ceremony ends explicitly instead of hanging forever.
- A restarted participant can recover from durable state.
- Phase timeout behavior is observable.
