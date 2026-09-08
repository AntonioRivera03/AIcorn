import { describe, expect, it } from "vitest";

import {
  normalizePersona,
  serializePersona,
  toPromptValue,
} from "@/features/persona/queries/persona-query-normalization";
import type { PersonaResponse } from "@/features/persona/queries/persona-query-normalization";
import { emptyDocument } from "@/lib/plate";
import type { Persona } from "@/types/types";
import type { Value } from "platejs";

const structuredPrompt: Value = [
  { type: "p", children: [{ text: "structured_token" }] },
];

const createResponse = (systemPrompt: unknown): PersonaResponse => ({
  ID: 7,
  Name: "Researcher",
  Harness: "codex",
  Model: "opencode-go/muse-spark-1.2-contributor",
  Agent: "",
  SystemPrompt: systemPrompt,
  AllowedTools: ["read_task"],
  TimeCreated: "2026-08-23T00:00:00Z",
  TimeModified: "2026-08-23T00:00:00Z",
});

describe("toPromptValue", () => {
  it("parses a serialized Plate document", () => {
    // Given a prompt returned as JSON text
    const raw = JSON.stringify(structuredPrompt);

    // When it crosses the persona query boundary
    const result = toPromptValue(raw);

    // Then the editor receives the structured document
    expect(result).toEqual(structuredPrompt);
  });

  it("keeps an already-parsed Plate document", () => {
    // Given a prompt that is already structured
    // When it crosses the persona query boundary
    const result = toPromptValue(structuredPrompt);

    // Then its node structure is preserved
    expect(result).toEqual(structuredPrompt);
  });

  it("wraps a legacy plain-text prompt in a paragraph", () => {
    // Given a legacy prompt stored before Plate parity
    const raw = "legacy_token";

    // When it crosses the persona query boundary
    const result = toPromptValue(raw);

    // Then no prompt content is discarded
    expect(result).toEqual([
      { type: "p", children: [{ text: "legacy_token" }] },
    ]);
  });

  it.each(["", "   ", "[]", " null ", `""`])(
    "normalizes the empty storage value %j",
    (raw) => {
      // Given an empty-compatible database representation
      // When it crosses the persona query boundary
      const result = toPromptValue(raw);

      // Then the editor receives its canonical empty document
      expect(result).toEqual(emptyDocument);
    },
  );

  it("normalizes malformed array JSON to an empty document", () => {
    // Given a corrupt value shaped like a serialized Plate array
    const raw = "[not-json";

    // When it crosses the persona query boundary
    const result = toPromptValue(raw);

    // Then invalid nodes cannot enter the editor
    expect(result).toEqual(emptyDocument);
  });

  it.each([null, undefined, { type: "p" }])(
    "normalizes the non-array value %j",
    (raw) => {
      // Given a non-document API value
      // When it crosses the persona query boundary
      const result = toPromptValue(raw);

      // Then the editor receives its canonical empty document
      expect(result).toEqual(emptyDocument);
    },
  );
});

describe("persona query normalization", () => {
  it("normalizes SystemPrompt while preserving persona metadata", () => {
    // Given a complete persona response with a serialized prompt
    const response = createResponse(JSON.stringify(structuredPrompt));

    // When the response is normalized
    const result = normalizePersona(response);

    // Then prompt structure and non-prompt fields are both retained
    expect(result).toEqual({ ...response, SystemPrompt: structuredPrompt });
  });

  it("serializes only SystemPrompt for persona writes", () => {
    // Given the editor-facing persona shape
    const persona: Persona = {
      ...createResponse(structuredPrompt),
      SystemPrompt: structuredPrompt,
    };

    // When the persona becomes an API payload
    const result = serializePersona(persona);

    // Then metadata stays intact and the prompt is JSON text
    expect(result).toEqual({
      ...persona,
      SystemPrompt: JSON.stringify(structuredPrompt),
    });
  });
});
