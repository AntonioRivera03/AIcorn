# Follow-up intent routing with Jev

The usual term for the “determiner” is an **intent classifier** or **intent router**. Its decision selects the next interaction mode; it does not grant execution authority. Aycorn always asks the user to confirm more work before moving the card or starting a work turn.

## Research, checked 2026-09-16

TypeSafe AI announced Jev on September 15, 2026, as a System One model for structured decisions. Its [launch post](https://typesafe.ai/blog/introducing-system-one-models-and-jev) advertises $0.042 per million input tokens, free output tokens, and roughly 70–500 ms latency. These are vendor claims, not Aycorn benchmarks. Availability was described as early access.

The [official quickstart](https://docs.typesafe.ai/introduction/quickstart) documents bearer authentication and `POST https://api.typesafe.ai/v1/systemone` with `model: "jev-latest"`, structured `state`, and typed `questions`. The [Choice primitive](https://docs.typesafe.ai/primitives/choice) returns a selected key, a probability distribution, and confidence. Restricted output types avoid free-form parsing, but do not guarantee that the semantic classification is correct.

## Implemented adapter

`server/internal/taskintent` supplies a small classifier interface and a Jev HTTP adapter. For a follow-up, the server sends the current message, task title, bounded task description, preceding request, and preceding answer. It does not send repository files or the entire conversation. The key is used by the server and excluded from the Codex child environment.

The choice criteria are:

- **question**: explain, clarify, discuss, or summarize existing results.
- **work**: make changes, implement/fix something, perform more checks, or conduct new research. Polite wording such as “can you fix it?” and mixed question/work requests count as work.
- **uncertain**: insufficient context or ambiguous intent.

A valid question result with question probability at least 0.90 submits a read-only turn. A work result with work probability at least 0.80 presents confirmation. Everything else presents the manual choice. These thresholds are application policy, not measured accuracy guarantees.

Timeouts (five seconds), provider errors, invalid response shapes, unknown choices, malformed probabilities, and missing credentials fall back to manual choice. No classifier result can automatically authorize work. The backend separately rechecks stage, latest conversation cursor, ownership, and dependencies when work is confirmed.

## Enable it

Obtain an API key from TypeSafe's access flow, then set `TYPESAFE_API_KEY` in the **Aycorn server process environment** and restart Aycorn. For a local development launch, export it in that shell before `make dev`; keep it out of repository files. A saved Codex login does not authenticate this separate provider.

Without the key, all follow-ups still work: the dialog offers **Ask only** or **Resume work**. Cancel keeps the draft and starts nothing. There is no paid classification request until a key is configured.

The HTTP contract, probability checks, fallback behavior, confirmation gate, and resumed session behavior have automated tests. The Codex session path was tested against the real installed driver. A real Jev request has **not** been verified because no TypeSafe API key was configured. Before relying on automatic question routing, evaluate it on representative task conversations, especially “why” questions containing work requests, ambiguous pronouns, and mixed requests; adjust thresholds using measured false classifications.
