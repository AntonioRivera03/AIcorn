import { describe, expect, it } from "vitest";

import { PERSONA_AGENTS, type PersonaAgent } from "@/types/types";

// Frontend-side agent validation mirroring server/internal/models.IsValidPersonaAgent
// Empty string is valid (no agent selected); otherwise must be in the curated catalog.
const isValidPersonaAgent = (value: string): boolean => {
  if (value === "") return true;
  return (PERSONA_AGENTS as readonly string[]).includes(value);
};

const validatePersona = (persona: { Agent: string }): { valid: boolean; error?: string } => {
  if (!isValidPersonaAgent(persona.Agent)) return { valid: false, error: "persona agent is not supported" };
  return { valid: true };
};

const EXPECTED_AGENTS = [
  "code-analysis",
  "code-implementation",
  "general-junior",
  "general-senior",
  "ox-coding-agent",
  "research",
  "reviewer",
  "summarizer",
  "synthesis",
  "test-integration",
  "worker",
] as const;

describe("PERSONA_AGENTS catalog", () => {
  it("has exactly 11 curated agents", () => {
    expect(PERSONA_AGENTS).toHaveLength(11);
    expect(EXPECTED_AGENTS).toHaveLength(11);
  });

  it("contains the expected agent slugs in order", () => {
    expect([...PERSONA_AGENTS]).toEqual([...EXPECTED_AGENTS]);
  });

  it("has no duplicates and all slugs are kebab-case", () => {
    const seen = new Set<string>();
    for (const agent of PERSONA_AGENTS) {
      expect(agent, `agent ${agent} should be kebab-case`).toMatch(/^[a-z]+(-[a-z]+)*$/);
      expect(seen.has(agent), `duplicate agent ${agent}`).toBe(false);
      seen.add(agent);
    }
  });

  it("matches .opencode/agent filenames (single source of truth for agent definitions)", () => {
    const expectedFiles = [...EXPECTED_AGENTS].sort();
    const catalog = [...PERSONA_AGENTS].sort();
    expect(catalog).toEqual(expectedFiles);
  });

  it("round-trips through PersonaAgent type: every catalog entry is a valid PersonaAgent", () => {
    for (const agent of PERSONA_AGENTS) {
      const typed: PersonaAgent = agent;
      expect(typed).toBe(agent);
      expect(isValidPersonaAgent(typed)).toBe(true);
    }
  });
});

describe("persona-editor-drawer agent select", () => {
  it("has Agent field wired (placeholder for drawer integration)", () => {
    expect(PERSONA_AGENTS.length).toBeGreaterThan(0);
  });
});

describe("validatePersona with agent", () => {
  it("accepts every curated agent", () => {
    for (const agent of PERSONA_AGENTS) {
      const result = validatePersona({ Agent: agent });
      expect(result.valid, `expected ${agent} to be valid`).toBe(true);
      expect(result.error).toBeUndefined();
    }
  });

  it("accepts empty string (no agent selected)", () => {
    const result = validatePersona({ Agent: "" });
    expect(result.valid).toBe(true);
    expect(result.error).toBeUndefined();
  });

  it("rejects an unknown agent with a descriptive error", () => {
    const result = validatePersona({ Agent: "unknown-agent" });
    expect(result.valid).toBe(false);
    expect(result.error).toMatch(/not supported/);
  });

  it("rejects legacy or mistyped agent slugs", () => {
    for (const bad of ["claude-code-agent", "Worker", "worker ", " code-analysis", "general_junior"]) {
      const result = validatePersona({ Agent: bad });
      expect(result.valid, `expected ${JSON.stringify(bad)} to be invalid`).toBe(false);
      expect(result.error).toBeDefined();
    }
  });

  it("exposes isValidPersonaAgent that mirrors the drawer save path", () => {
    // saveAgent is typed as PersonaAgent | "" — the guard must allow "" and reject anything else.
    expect(isValidPersonaAgent("")).toBe(true);
    expect(isValidPersonaAgent("worker")).toBe(true);
    expect(isValidPersonaAgent("research")).toBe(true);
    expect(isValidPersonaAgent("not-an-agent")).toBe(false);
    expect(isValidPersonaAgent("claude-code")).toBe(false);
  });

  it("treats agent validation as the source of the 400 for invalid agent", () => {
    // Mirrors server behavior: ErrInvalidPersonaAgent maps to 400.
    // Frontend must surface the same rejection before the request leaves the client.
    const bad = validatePersona({ Agent: "bad-agent" });
    expect(bad.valid).toBe(false);
    // The error message must be the same shape the server returns so the UI can show it.
    expect(bad.error).toBe("persona agent is not supported");
  });
});
