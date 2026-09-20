# Agent performance evaluations

Issue: https://github.com/AntonioRivera03/AIcorn/issues/17

The versioned internal suite in `server/internal/evals/cases.json` runs **real Codex turns through Aycorn's production harness and MCP server**. It is separate from unit/protocol tests and opts in with `-live`; it uses the installed Codex login and consumes model usage.

## Run

From `server/`, with Go, Node, Python 3, and a logged-in Codex CLI installed:

```sh
go build -o /tmp/aycorn-eval-mcp ./cmd/mcp
go run ./cmd/eval -list
go run ./cmd/eval -live -mcp /tmp/aycorn-eval-mcp -model gpt-5.6-sol -variant both -repeat 1 -output ../evals/results/my-experiment
```

Use `-codex /absolute/path/to/codex` to select an installation. `-timeout 120` limits each agent turn. `-variant baseline` or `-variant verify-first` runs one treatment. For stronger evidence, repeat the paired experiment with `-repeat 3` or more. Never compare different suite versions as a prompt-only improvement.

Each case gets a disposable SQLite database, a scoped running task, and a separate workspace. No application database, real task, repository checkout, or GitHub state is modified. Temporary fixtures are removed afterward. Codex retains native session history under the local Codex home, as it does for application runs. Original files and graders are controlled by the runner; agent statements do not determine whether a case passed.

## Fixed suite v2

Eight coding cases require `solve(value)` in Python: slug normalization, stable priority ordering, task mention filtering, revision-aware patches, configured workflow transitions, bounded retries, dependency ordering, and inclusive calendar dates. Each has several deterministic assertions, including empty inputs, stale revisions, cycles, large retry counts, and leap dates.

Four task-manager cases exercise real Aycorn MCP: read the exact title/priority, attach a PR, attach a branch, and attach two spellings of the same PR without duplicating it. These inspect the resulting database and reject unrequested title, body, priority, or stage changes. GitHub URLs are fixture references; the agent is instructed not to access GitHub.

This is a small internal regression suite, not a measure of arbitrary software engineering ability. It does not yet grade UI design, multi-day Conductor workflows, or production deployments. Hidden assertions live outside the agent workspace, but this is not a hardened adversarial benchmark: a local process can access its host, and the graders run candidate Python code like an ordinary test suite.

## Experiment

The baseline prompt asks the agent to complete the task concisely. The `verify-first` prompt adds explicit instructions to read the current ticket, identify acceptance conditions, exercise boundary cases, and verify stored tool results. Model, task set, grader, adapter, timeout, and sandbox stay the same. The runner interleaves baseline and treatment for each case. Exact prompts are saved in the report.

JSON records suite version and SHA-256, source commit and tracked-diff hash, model, engine version, exact prompts, timestamps, per-case verdicts, errors, latency, native usage, aggregate solve rates, and treatment-minus-baseline percentage points. CSV provides the per-case measurements. Reports checkpoint after every case; `completedAt` and `planned` distinguish completed from interrupted experiments. A failed/timed-out agent cannot pass even if it left a correct artifact. Partial runs are not a completed experiment.

`tokens: null` means the provider did not report usage. `costUsd: null` means dollar cost is unavailable: subscription usage must not be presented as free or priced using invented API rates. Reported tokens include cached input separately and preserve raw native usage. Latency includes setup and model/tool execution, excludes grading, and is affected by cache and service load. A single small run is descriptive, not statistically significant evidence of an improvement. Zero or negative deltas are valid results.

Run grading/report regression tests without a model:

```sh
go test ./internal/evals
```

These verify that broken starter implementations fail, known-correct behavior passes, a claimed success without a saved link fails, unwanted task edits fail, and missing token usage stays unknown.

## Recorded measurements

The [September 16 experiment report](../evals/results/README.md) includes completed JSON/CSV baselines, an interleaved prompt comparison, and a measured MCP configuration fix. The prompt change remained at 9/12 passes; the configuration fix reached 10/12, with all scoped link writes passing and two timeouts retained as failures. The report includes the isolated patch, executable fingerprints, reproduction commands, and the interrupted pilot for auditability.
