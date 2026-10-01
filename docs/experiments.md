# Experiment Plan

The result files contain one local run per scenario for each implementation. Mock and real DKG records have different recovery and threshold behavior; compare them by scenario, not as identical protocol executions.

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

These are single local observations and do not establish latency distributions or a cryptographic security property. Mock E4 specifically reflects the mock state machine's all-share participation rule despite `threshold=3`.

## Real DKG process results

On 2026-10-01, `dkgctl real-run --scenario E0..E6` produced the linked JSON files from revision `f5e38932c9d40a116422d399c3570c462df917bb` (Go 1.25.0, darwin/arm64). Each run started four separate loopback TCP participant processes using drand/kyber `v1.3.2`, Ed25519, threshold 3, and fresh cryptographic randomness. The controller forwarded signed deal, response, and justification packets as needed. `group_key_agreement=consistent` means every finalized participant in that run returned the same group public key hash; `not_observed` means nobody finalized.

| Scenario | Record | Terminal result | Total duration | Observation |
|---|---|---|---:|---|
| E0 | [JSON](../results/real/2026-10-01/E0.json) | Completed | 24 ms | Four finalized; same group public key |
| E1 | [JSON](../results/real/2026-10-01/E1.json) | Completed | 19 ms | Duplicate p2 deal to p1 was idempotent; four finalized |
| E2 | [JSON](../results/real/2026-10-01/E2.json) | Completed | 17 ms | Stale-session deal rejected; four finalized |
| E3 | [JSON](../results/real/2026-10-01/E3.json) | Completed after abort | 49 ms | p1 killed after one peer deal; old ceremony aborted; four new processes finalized under a fresh nonce (44 ms from crash) |
| E4 | [JSON](../results/real/2026-10-01/E4.json) | Completed at threshold | 15 ms | p4 killed after deal generation; p1–p3 finalized with the same 3-member qualified set |
| E5 | [JSON](../results/real/2026-10-01/E5.json) | Aborted | 13 ms | 2:2 partition; library reported fewer than 3 valid deals at all four nodes |
| E6 | [JSON](../results/real/2026-10-01/E6.json) | Completed at threshold | 121 ms | p4 outbound deals held; p1–p3 finalized; three deliveries rejected after a 102 ms hold |

Real `total_duration_ms` starts after the participant processes become ready, includes controller setup and packet handling, and excludes initial process startup. E3 includes the old-session abort and new process startup; `recovery_duration_ms` measures from p1's process kill to completion of the new ceremony, while `fresh_run_duration_ms` measures only the second ceremony. `phase_duration_ms` is controller-observed setup/deal generation, deal delivery, response/justification handling, and E6's post-terminal hold; it is not a cryptographic benchmark. E6 holds p4's outbound packets until recipients terminate and then waits the configured `--hold-duration` (100 ms by default) before late delivery. This measures the injected hold, not a network latency distribution or a DKG phase timeout. The real runner labels an execution `completed` when at least the threshold participants finalize with one group public key; it labels E5 `aborted` when the library reports insufficient valid deals. Counts describe scheduled or observed events in that single run.

The mock WAL resumes the **same** session in E3. The real Kyber path cannot safely replay its in-memory private polynomial from that WAL, so real E3 aborts and starts a **new** session. The differing E4 results show the mock all-share rule and Kyber's threshold qualification rule; neither result should be generalized beyond the recorded local setup. Library choice and security assumptions are documented in [Real DKG Library Selection](dkg-library-selection.md).

## M6 mutual TLS rerun

On 2026-10-01, the clean binary from `b5b7da0f41541461e5a131fe5f604ae84bb85171` ran all seven four-process fault schedules again. Each real participant RPC used a fresh local mutual TLS certificate set. E3 used the normal ceremony supervisor: after p1 died, it stopped the old processes, started four new processes with new keys and nonce, rejected one old signed deal, and finalized the new ceremony. These results are one local run per case.

| Scenario | Record | Result | Duration | Main observation |
|---|---|---|---:|---|
| E0 | [JSON](../results/real/2026-10-01-m6/E0.json) | Completed | 55 ms | Four finalized |
| E1 | [JSON](../results/real/2026-10-01-m6/E1.json) | Completed | 45 ms | Duplicate treated idempotently; four finalized |
| E2 | [JSON](../results/real/2026-10-01-m6/E2.json) | Completed | 42 ms | Stale deal rejected; four finalized |
| E3 | [JSON](../results/real/2026-10-01-m6/E3.json) | Completed after abort | 143 ms | One abort, four restarted processes, old deal rejected, four finalized; 89 ms measured from crash to completion |
| E4 | [JSON](../results/real/2026-10-01-m6/E4.json) | Completed at threshold | 43 ms | Three finalized with one group key |
| E5 | [JSON](../results/real/2026-10-01-m6/E5.json) | Aborted | 37 ms | No participant finalized in the 2:2 split |
| E6 | [JSON](../results/real/2026-10-01-m6/E6.json) | Completed at threshold | 392 ms | Three finalized; held packets rejected after terminal state |

A separate [normal ceremony](../results/real/2026-10-01-m6/ceremony.json) finalized all four participants. Its [controller journal](../results/real/2026-10-01-m6/ceremony-journal.jsonl) records one started and one finalized decision with the same nonce hash. The ceremony's `total_duration_ms` (330 ms) includes local process startup and certificate generation, while ordinary E0–E2 and E4–E6 `real-run` durations start after process readiness. The E3 duration includes both attempts and process restarts. These timings are not comparable as a benchmark.
