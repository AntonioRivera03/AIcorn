export type AIIntent = "ask" | "plan" | "implement" | "review";
export type AIRunInput = {
  intent: AIIntent;
  instruction: string;
  presetId: number;
  useRepository: boolean;
};
export type AISettings = {
  model: string;
  executable: string;
  timeoutSeconds: number;
};
export type AISettingsResponse = {
	providers: { id: string; name: string; enabled: boolean; reason?: string }[];
  settings: AISettings;
  engine: {
    ready: boolean;
    executable: string;
    version: string;
    error?: string;
  };
};
export type AIRunRequest = {
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
