// One step of Chatter's work in a turn (server: harness.Activity).
export type ChatActivity = {
  id: string;
  kind: "thinking" | "tool";
  tool?: string;
  arguments?: Record<string, unknown>;
  result?: unknown;
  text?: string;
  status: "inProgress" | "completed" | "failed";
  error?: string;
  startedAt?: number;
  durationMs?: number;
};

export type TurnStatus =
  | "pending"
  | "running"
  | "canceling"
  | "completed"
  | "failed"
  | "canceled"
  | "interrupted";

export type ChatTurn = {
  id: number;
  conversationId: number;
  message: string;
  status: TurnStatus;
  output: string;
  progress: string;
  error: string;
  createdAt: string;
  finishedAt?: string;
  activity: ChatActivity[] | null;
};

export type ChatConversation = {
  id: number;
  projectId: number;
  title: string;
  updatedAt: string;
  turns: ChatTurn[];
};

export type ChatSummary = {
  id: number;
  title: string;
  updatedAt: string;
  status: TurnStatus | "";
};

export const isWorking = (status: string) =>
  status === "pending" || status === "running" || status === "canceling";

// SQLite's UTC "YYYY-MM-DD HH:MM:SS", read as UTC.
export const serverTime = (value: string) =>
  new Date(value.includes("T") ? value : `${value.replace(" ", "T")}Z`);
