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
| E0 | Normal run | 4 participants, threshold 3 | Completes; invalid transition count is 0 | Planned |
| E1 | Duplicate delivery | Duplicate protocol messages | Duplicates observed; duplicate transition count is 0; ceremony remains valid | Planned |
| E2 | Stale delivery | Delay a message until the receiver advances beyond its valid state | Reject stale message; current state unchanged | Planned |
| E3 | Crash during SHARE | Persist some SHARE state, crash, restart | Replay durable state; resume same session; no re-application; record recovery duration | Planned |
| E4 | One unavailable participant | 4 participants, threshold 3; one unavailable | Follow selected DKG protocol's actual threshold and liveness rules | Planned |
| E5 | Threshold-deficient partition | 4 participants, threshold 3; partition A,B | C,D | Neither side finalizes while partitioned | Planned |
| E6 | Slow participant | Inject latency into one participant or link | Phase deadline is observable; timeout does not corrupt state | Planned |

Healing the E5 partition and observing recovery is a separate optional run and must be recorded separately.

## Results log

The E0 integration test is implemented but has not been executed. No measured experiment results have been recorded yet.
