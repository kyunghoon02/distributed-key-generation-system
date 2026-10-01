# Experiment Plan

Experiment results are intentionally empty until each run has actually been executed. Do not copy expected outcomes into measured results.

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
| E0 | Normal run | 4 participants, threshold 3 | Completes; invalid transition count is 0 | Normal-flow test passed; measurements pending |
| E1 | Duplicate delivery | Duplicate protocol messages | Duplicates observed; duplicate transition count is 0; ceremony remains valid | Mock SHARE semantics tested; experiment pending |
| E2 | Stale delivery | Delay a message until the receiver advances beyond its valid state | Reject stale message; current state unchanged | Mock SHARE semantics tested; experiment pending |
| E3 | Crash during SHARE | Persist some SHARE state, crash, restart | Replay durable state; resume same session; no re-application; record recovery duration | Planned |
| E4 | One unavailable participant | 4 participants, threshold 3; one unavailable | Follow selected DKG protocol's actual threshold and liveness rules | Planned |
| E5 | Threshold-deficient partition | 4 participants, threshold 3; partition A,B | C,D | Neither side finalizes while partitioned | Planned |
| E6 | Slow participant | Inject latency into one participant or link | Phase deadline is observable; timeout does not corrupt state | Planned |

Healing the E5 partition and observing recovery is a separate optional run and must be recorded separately.

## Results log

On 2026-10-01, the E0 four-process normal-flow test and CLI run completed successfully. M1 unit and local TCP tests also passed for duplicate SHARE delivery and stale round rejection. These are test results, not completed E0–E2 experiment records: fault schedules and the required timing and counter fields have not been captured yet.
