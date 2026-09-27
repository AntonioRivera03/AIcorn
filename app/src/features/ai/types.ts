import type { PersonaHarness } from "@/types/types";

export type AIIntent = "ask" | "plan" | "implement" | "review";
export type AIRunInput = {
  intent: AIIntent;
  instruction: string;
  presetId: number;
  useRepository: boolean;
};
export type AISettings = {
  harness: PersonaHarness;
  model: string;
  executable: string;
  timeoutSeconds: number;
};
// A harness the workspace can choose. `runnable` is false while Aycorn has no
// adapter for it, and `note` says why.
export type HarnessInfo = {
  id: PersonaHarness;
  name: string;
  runnable: boolean;
  note?: string;
};
export type HarnessModel = {
  id: string;
  name: string;
  description?: string;
  default?: boolean;
};
// `live` is true when the harness listed its own models, false for Aycorn's
// fixed fallback list.
export type HarnessModels = { models: HarnessModel[]; live: boolean };
export type AISettingsResponse = {
  providers: HarnessInfo[];
  settings: AISettings;
  engine: {
    ready: boolean;
    executable: string;
    version: string;
    error?: string;
  };
};
export type AIRunRequest = {
  taskSession?: { role: string; mode: "question" | "work"; settings?: { workingStage: number; completionStage: number } };
  chat?: { clientKey: string; previousJob: number; sessionId?: string };
  conductor?: { phase: "planning" | "working" };
  key: string;
  intent: AIIntent;
  instruction: string;
  taskName: string;
  taskBody: string;
  presetName?: string;
  systemPrompt?: string;
  model: string;
  executable: string;
  engineVersion: string;
  engine?: string;
  agentId?: number;
  repoPath?: string;
  timeoutSeconds: number;
};
export type AIArtifacts = {
  turnDiff?: string;
  turnFiles?: string[];
	provider?: string;
	sessionId?: string;
	turnId?: string;
  workspace?: string;
  branch?: string;
  baseCommit?: string;
  diff?: string;
  files?: string[];
  warning?: string;
};
