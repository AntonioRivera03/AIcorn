# Recorded experiments — 2026-09-16

These are real model runs through Aycorn's Codex harness and scoped MCP server, using disposable tasks and workspaces. The completed experiments use suite **v2**, `gpt-5.6-sol`, Codex CLI `0.153.4`, one trial per case, and a 120-second turn timeout. See [methodology and run instructions](../../Documentation/evals.md).

| Configuration | Passed | Solve rate | Mean latency | Reported tokens | Dollar cost |
| --- | ---: | ---: | ---: | ---: | --- |
| Original harness, baseline prompt | 9/12 | 75.0% | 33.72 s | 1,604,178 | Unavailable |
| Original harness, verify-first prompt | 9/12 | 75.0% | 38.73 s | 1,863,384 | Unavailable |
| Scoped MCP approval fix, baseline prompt | 10/12 | 83.3% | 45.68 s | 1,588,179 | Unavailable |

Tokens include cached input; cached input is a subset of input, not an additional charge. The native subscription session does not report attributable dollar cost. The reports preserve raw usage and use `costUsd: null`.

## Findings

The [paired prompt experiment](2026-09-16-prompt-experiment.json) completed all 24 planned turns. Both prompts passed all eight coding cases and the read-context case, but failed the three MCP link writes. The prompt change therefore produced **0 percentage points** of solve-rate improvement and used more reported tokens and time in this run. Its [CSV](2026-09-16-prompt-experiment.csv) retains every verdict.

The write failures exposed an integration bug: the session's `approval_policy = never` did not grant approval to its scoped MCP write tools. The [routing patch](2026-09-16-routing-change.patch) explicitly sets `approval_mode = approve` for each tool already exposed to that session. Server-side project, task ownership, and live-run checks still apply. This does not grant GitHub access or approval to unrelated tools.

The [routing experiment](2026-09-16-scoped-routing.json) repeated all 12 cases with the baseline prompt and the same frozen MCP binary, applying only that session configuration change to the evaluated harness. All four task-manager cases passed, including the three writes that previously failed. Overall solve rate increased **8.33 percentage points**, from 9/12 to 10/12. `retry-delay` and `calendar-span` both hit the 120-second deadline; their artifacts passed the grader, but the agent did not finish its turn, so both remain failures. No selective reruns replaced them. The [CSV](2026-09-16-scoped-routing.csv) includes these errors and their latency.

These are small, single-trial measurements, not evidence of a general or statistically significant model improvement. The routing run happened after the interleaved prompt experiment; cache state and service load may differ. The measured increase supports the specific MCP integration fix, while the timeout failures remain visible.

## Provenance and reproduction

[experiment-manifest.json](2026-09-16-experiment-manifest.json) records report and executable hashes, reproduction commands with model and timeout defaults made explicit, source revisions, the routing patch, and the comparison. Each report also records its dataset hash, prompts, native engine version, source revision, and tracked-diff hash. Reports describe the checkout at execution time; the executable fingerprints identify the frozen binaries actually run.

The original runner was built before the approval fix. The routing runner was built from an isolated checkout at `9234534`, with only the supplied patch applied. Harness, eval-runner, eval-dataset, and MCP source trees are unchanged between the baseline report's `7164cd0` revision and `9234534`; the intervening commit adds document uploads. Both measurements used the same `/tmp/aycorn-eval-mcp` executable. The production fix is included in `3000657` alongside the project chat implementation.

To reproduce the configuration comparison, build the eval runner and MCP executable from the original source, preserve the MCP executable, and run the baseline. Apply the recorded patch in an isolated checkout at `9234534`, rebuild only the eval runner, and run the same suite/model/prompt/timeout against the preserved MCP executable. Follow the commands in the manifest and use fresh output paths. Temporary executable paths are machine-local; the hashes remain as provenance after cleanup. Do not expect identical model output or latency.

## Interrupted pilot

[pilot-v1.json](2026-09-16-pilot-v1.json) and its [CSV](2026-09-16-pilot-v1.csv) preserve an interrupted four-turn pilot out of 24 planned turns. Its slug instructions were ambiguous about uppercase input. The prompt was clarified and the suite version bumped **before** starting the completed v2 baseline. The pilot has no `completedAt`, includes a canceled turn, and is excluded from all comparisons above. Its partial summary is not a completed baseline and must not be compared with suite v2.
