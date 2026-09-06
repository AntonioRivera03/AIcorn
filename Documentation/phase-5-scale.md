> Superseded for new AI work by [the explicit-run redesign](ai-integration-redesign-review.md). Retained as historical phase notes; stage bindings and assignee-triggered execution are being retired.

# Phase 5 — Scale With Evidence

Status: Not started

Everything here is a response to pain actually felt running Phases 3–4, not a preemptive build. Don't start this phase until something below is a real, observed problem.

---

## Candidate work (pull items in only once they hurt)

- Raise `maxConcurrentJobs` past 1 — watch for 529s from Claude Code as you go.
- Retry with exponential backoff and jitter for transient failures.
- A `failed` terminal state on `agent_job` with manual requeue from the UI. A full dead-letter-queue table is over-specified for a single-user app — don't build one preemptively.
- A run timeline / history view in the task drawer (beyond the single latest `agent_run`).
- A second `Harness` implementation (Crush, once its programmatic API is verified by hand) — see the open question in the main plan artifact.
- Backend tests for the two concurrency-critical queries: the job-claim query (Phase 3) and the transition CAS query (Phase 0) — the only concurrency-critical SQL in the whole plan, and the cheapest place to add test coverage given the app currently has none.

## Done when

There's no fixed "done" for this phase — it's ongoing hardening driven by observed usage. Track individual items as they're picked up rather than treating the whole phase as one unit of work.
