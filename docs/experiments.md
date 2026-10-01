# Experiment Plan

The result files below contain observations from one local mock-runtime run per scenario. Do not copy expected outcomes into measured results or treat these files as real DKG evidence.

## Required result fields

Record the following for every run:

- Experiment ID and software revision
- Participant count
- Threshold
- Injected fault and injection point
- Expected invariant
- Terminal result (completed, aborted, or timed out)
- Total completion time
- Per-phase duration
- Retry count
- Timeout count
- Duplicate count
- Stale-message count
- Recovery duration, when applicable
- Machine-readable result (JSON) once M4 is implemented

## Experiments

| ID | Scenario | Setup / fault | Expected invariant or behavior | Status |
|---|---|---|---|---|
| E0 | Normal run | 4 participants, threshold 3 | Completes; invalid transition count is 0 | [Completed](../results/mock/2026-10-01/E0.json) |
| E1 | Duplicate delivery | Repeat p2 → p1 SHARE | Duplicate transition count is 0; ceremony remains valid | [Completed](../results/mock/2026-10-01/E1.json) |
| E2 | Stale delivery | Deliver old-round SHARE before normal traffic | Reject stale message; current state unchanged | [Completed](../results/mock/2026-10-01/E2.json) |
| E3 | Crash during SHARE | Persist one peer SHARE at p1, kill, restart | Replay durable state; resume same session; no re-application | [Completed](../results/mock/2026-10-01/E3.json) |
| E4 | One unavailable participant | 4 participants, threshold 3; kill p4 after begin | Follow selected protocol's participation rules | [Timed out](../results/mock/2026-10-01/E4.json) |
| E5 | Threshold-deficient partition | 4 participants, threshold 3; split p1,p2 from p3,p4 | Neither side finalizes while partitioned | [Timed out](../results/mock/2026-10-01/E5.json) |
| E6 | Slow participant | Hold p4 outbound SHARE past deadline | Timeout is terminal; late SHARE cannot mutate state | [Timed out](../results/mock/2026-10-01/E6.json) |

Healing the E5 partition and observing recovery is a separate optional run and must be recorded separately.

## Results log

On 2026-10-01, `dkgctl experiment --scenario E0..E6 --phase-deadline 300ms` produced the linked JSON files from revision `7737678e9d43295dad67c9318dbdb0605a24031f` (Go 1.24.4, darwin/arm64). `total_duration_ms` starts before the first `begin` request and ends after terminal status collection; process startup is excluded. `phase_duration_ms` measures controller-observed initialization, SHARE delivery/deadline, and VERIFY/finalize intervals, not internal cryptographic phase timing. Counts describe runner-injected or observed events. A missing `verify` duration means the ceremony timed out before finalization.

| Scenario | Terminal result | Total duration | Key observation |
|---|---|---:|---|
| E0 | Completed | 90 ms | All four participants finalized |
| E1 | Completed | 68 ms | One duplicate; no extra transition |
| E2 | Completed | 71 ms | One stale old-round SHARE rejected |
| E3 | Completed | 100 ms | Replay matched pre-crash state; recovery response in 28 ms |
| E4 | Timed out | 331 ms | Three responsive participants received 3/4; the mock runtime requires all four |
| E5 | Timed out | 338 ms | Each partition member received 2/4; none finalized |
| E6 | Timed out | 340 ms | Three delayed SHAREs were rejected after timeout |

These are single local observations and do not establish latency distributions, a cryptographic security property, or liveness under a selected real DKG protocol. E4 specifically reflects the current mock state machine's all-share participation rule despite `threshold=3`.
