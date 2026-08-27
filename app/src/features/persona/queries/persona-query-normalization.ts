import { toValidBody } from "@/lib/plate";
import type { Persona } from "@/types/types";
import type { Value } from "platejs";

export type PersonaResponse = Omit<Persona, "SystemPrompt"> & {
  readonly SystemPrompt: unknown;
};

const tryParse = (value: string): unknown => {
  try {
    return JSON.parse(value);
  } catch (error) {
    if (error instanceof SyntaxError) return null;
    throw error;
  }
};

export const toPromptValue = (raw: unknown): Value => {
  if (typeof raw !== "string") return toValidBody(raw);

  const trimmed = raw.trim();
  if (
    trimmed === "" ||
    trimmed === "[]" ||
    trimmed === "null" ||
    trimmed === `""`
  ) {
    return toValidBody([]);
  }

  const parsed = tryParse(raw);
  if (Array.isArray(parsed)) return toValidBody(parsed);
  if (!trimmed.startsWith("[")) {
    return [{ type: "p", children: [{ text: raw }] }];
  }
  return toValidBody(parsed);
};

export const normalizePersona = (raw: PersonaResponse): Persona => ({
  ...raw,
  SystemPrompt: toPromptValue(raw.SystemPrompt),
});

export const serializePersona = (persona: Persona): Record<string, unknown> => ({
  ...persona,
  SystemPrompt: JSON.stringify(persona.SystemPrompt),
});
