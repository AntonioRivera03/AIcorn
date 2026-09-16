import { useEffect, useId, useRef, useState } from "react";
import { useTaskOwnership } from "@/features/ai/queries/use-task-ownership";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Copy, MessageSquare, Send, Square } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Checkbox } from "@/components/ui/checkbox";
import { Badge } from "@/components/ui/badge";
import { AIMarkdown } from "@/features/ai/ai-markdown";
import {
  useAIContext,
  useAISettings,
  useAIMutations,
} from "@/features/ai/queries/use-ai";
import { usePersonasQuery } from "@/features/persona/queries/use-personas-query";
import {
  isAgentWorking,
  useAgentJobs,
  type AgentJob,
  type AgentRun,
} from "@/features/agentJob/queries/useAgentJobs";

type MessageInput = {
  key: string;
  message: string;
  presetId: number;
  mode: "ask" | "edit";
  useRepository: boolean;
  newConversation: boolean;
};
function readDraft(key: string) {
  try {
    return localStorage.getItem(key) ?? "";
  } catch {
    return "";
  }
}
const selectStyle =
  "h-9 max-w-full rounded-md border border-input bg-background px-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50";

export function TicketChat({ taskId }: { taskId: number }) {
  const history = useAgentJobs(taskId);
  const settings = useAISettings();
  const agents = usePersonasQuery();
  const { project } = useAIContext(taskId);
  const ownership = useTaskOwnership(project?.ID);
  const owner = ownership.data?.find((item) => item.taskId === taskId);
  const { cancel } = useAIMutations(taskId);
  const client = useQueryClient();
  const composerId = useId();
  const draftKey = `aycorn-chat-draft-${taskId}`;
  const [message, setMessage] = useState(() => readDraft(draftKey));
  const [mode, setMode] = useState<"ask" | "edit">("ask");
  const [presetId, setPresetId] = useState<number | null>(null);
  const [repositoryChoice, setRepositoryChoice] = useState<boolean | null>(
    null,
  );
  const [newConversation, setNewConversation] = useState(false);
  const requestKey = useRef<{ signature: string; key: string } | null>(null);
  const turns = (history.data?.jobs ?? [])
    .filter((j) => j.request?.chat)
    .sort((a, b) => a.id - b.id);
  const last = turns.at(-1);
  const selectedAgent = presetId ?? last?.request?.agentId ?? 0;
  const useRepository =
    mode === "edit" ||
    (repositoryChoice ??
      (last ? !!last.request?.repoPath : !!project?.RepoPath));
  const active = (history.data?.jobs ?? []).find((j) => isAgentWorking([j]));
  const session = [...(history.data?.runs ?? [])]
    .sort((a, b) => b.id - a.id)
    .find((r) => turns.some((j) => j.id === r.job) && r.artifacts?.sessionId)
    ?.artifacts?.sessionId;
  useEffect(() => {
    try {
      if (message) localStorage.setItem(draftKey, message);
      else localStorage.removeItem(draftKey);
    } catch {
      /* The composer still works if storage is unavailable. */
    }
  }, [draftKey, message]);
  const send = useMutation({
    mutationFn: async (input: MessageInput) => {
      const response = await fetch(`/api/ai/tasks/${taskId}/chat`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(input),
      });
      if (!response.ok) throw new Error((await response.text()).trim());
      return response.json() as Promise<AgentJob>;
    },
    onSuccess: (job, input) => {
      requestKey.current = null;
      setMessage((draft) => (draft === input.message ? "" : draft));
      setNewConversation(false);
      client.setQueryData<{ jobs: AgentJob[]; runs: AgentRun[] }>(
        ["agent-jobs", taskId],
        (old) =>
          old
            ? {
                ...old,
                jobs: [...old.jobs.filter((j) => j.id !== job.id), job],
              }
            : { jobs: [job], runs: [] },
      );
      void client.invalidateQueries({ queryKey: ["agent-jobs", taskId] });
      void client.invalidateQueries({ queryKey: ["active-agent-jobs"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });
  const revision = history.data?.jobs
    .map((j) => `${j.id}:${j.status}`)
    .join("|");
  useEffect(() => {
    void client.invalidateQueries({ queryKey: ["task-branches", taskId] });
  }, [client, taskId, revision]);
  const busy = !!active || send.isPending;
  const blocked =
    busy ||
    !!owner ||
    ownership.isPending ||
    ownership.isError ||
    !message.trim() ||
    !settings.data?.engine.ready ||
    history.isPending ||
    history.isError ||
    taskId === 0;
  function submit() {
    if (blocked) return;
    const payload = {
      message,
      presetId: selectedAgent,
      mode,
      useRepository,
      newConversation,
    };
    const signature = JSON.stringify(payload);
    if (requestKey.current?.signature !== signature)
      requestKey.current = { signature, key: crypto.randomUUID() };
    send.mutate({ ...payload, key: requestKey.current.key });
  }
  return (
    <section
      aria-label="Ticket chat"
      className="min-w-0 space-y-5 rounded-xl border bg-card p-4 sm:p-5"
      data-vaul-no-drag
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="flex items-center gap-2 font-semibold">
            <MessageSquare className="size-4" />
            Ticket chat
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            You direct each message. Chat leaves this ticket’s title, body,
            assignee and stage under your control.
          </p>
        </div>
        {session && (
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              void navigator.clipboard
                .writeText(`codex resume ${session}`)
                .then(
                  () => toast.success("Resume command copied"),
                  () => toast.error("Could not copy"),
                );
            }}
          >
            <Copy className="size-3" />
            Resume in Codex
          </Button>
        )}
      </div>
      {owner && !active && (
        <p role="status" className="text-sm text-muted-foreground">
          {owner.name} is managing this task ({owner.state}). Wait for it to
          finish or release the task before sending another agent.
        </p>
      )}
      {ownership.isError && (
        <p role="alert" className="text-sm text-destructive">
          {ownership.error.message}
        </p>
      )}
      {history.isPending ? (
        <p role="status" className="text-sm text-muted-foreground">
          Loading conversation…
        </p>
      ) : history.isError ? (
        <p role="alert" className="text-sm text-destructive">
          Could not load messages.{" "}
          <Button variant="link" onClick={() => void history.refetch()}>
            Retry
          </Button>
        </p>
      ) : turns.length === 0 ? (
        <p className="rounded-lg bg-muted p-5 text-sm text-muted-foreground">
          Start a conversation about this ticket. Ask mode reads and explains;
          Edit mode can change files in an isolated project branch when you
          request it.
        </p>
      ) : (
        <ol className="space-y-6">
          {turns.map((j) => (
            <ChatMessage
              key={j.id}
              job={j}
              run={
                history.data?.runs
                  .filter((r) => r.job === j.id)
                  .sort((a, b) => b.id - a.id)[0]
              }
            />
          ))}
        </ol>
      )}
      {active && (
        <div
          role="status"
          className="flex flex-wrap items-center justify-between gap-2 text-sm"
        >
          <span>
            {active.progress ||
              (active.status === "pending"
                ? "Queued…"
                : active.status === "canceling"
                  ? "Stopping…"
                  : "Working…")}
          </span>
          <Button
            size="sm"
            variant="outline"
            disabled={cancel.isPending || active.status === "canceling"}
            onClick={() => cancel.mutate(active.id)}
          >
            <Square className="size-3" />
            Stop
          </Button>
        </div>
      )}
      <div className="space-y-3 border-t pt-4">
        <div className="flex flex-wrap gap-3">
          <div className="space-y-1">
            <Label htmlFor={`${composerId}-agent`}>Agent</Label>
            <select
              id={`${composerId}-agent`}
              className={selectStyle}
              value={selectedAgent}
              disabled={busy || agents.isPending}
              onChange={(e) => setPresetId(Number(e.target.value))}
            >
              <option value={0}>Codex · default model</option>
              {selectedAgent > 0 &&
                !agents.data?.some((a) => a.ID === selectedAgent) && (
                  <option value={selectedAgent}>Agent unavailable</option>
                )}
              {agents.data?.map((a) => (
                <option key={a.ID} value={a.ID}>
                  {a.Name || "Untitled"} · {a.Model}
                </option>
              ))}
            </select>
          </div>
          <div className="space-y-1">
            <Label htmlFor={`${composerId}-mode`}>Mode</Label>
            <select
              id={`${composerId}-mode`}
              className={selectStyle}
              value={mode}
              disabled={busy}
              onChange={(e) => setMode(e.target.value as "ask" | "edit")}
            >
              <option value="ask">Ask · read only</option>
              <option value="edit" disabled={!project?.RepoPath}>
                Edit files
              </option>
            </select>
          </div>
        </div>
        <div className="flex flex-wrap gap-4 text-sm">
          <div className="flex items-center gap-2">
            <Checkbox
              id={`${composerId}-repo`}
              checked={useRepository}
              disabled={busy || mode === "edit" || !project?.RepoPath}
              onCheckedChange={(v) => setRepositoryChoice(v === true)}
            />
            <Label htmlFor={`${composerId}-repo`}>Use project repository</Label>
          </div>
          {turns.length > 0 && (
            <div className="flex items-center gap-2">
              <Checkbox
                id={`${composerId}-fresh`}
                checked={newConversation}
                disabled={busy}
                onCheckedChange={(v) => setNewConversation(v === true)}
              />
              <Label htmlFor={`${composerId}-fresh`}>
                Start a new conversation with this message
              </Label>
            </div>
          )}
        </div>
        {newConversation && (
          <p className="text-xs text-muted-foreground">
            Earlier messages and branches stay here. Your next message starts a
            separate Codex conversation and workspace.
          </p>
        )}
        <Label htmlFor={composerId}>Message</Label>
        <Textarea
          id={composerId}
          placeholder="What would you like to work on?"
          value={message}
          maxLength={32000}
          className="min-h-28 resize-y"
          onChange={(e) => setMessage(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
              e.preventDefault();
              submit();
            }
          }}
        />
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-xs text-muted-foreground">
            Ctrl/⌘ + Enter to send. Drafts stay on this device.
          </p>
          <Button disabled={blocked} onClick={submit}>
            <Send className="size-4" />
            {send.isPending ? "Sending…" : "Send message"}
          </Button>
        </div>
        {send.error && (
          <p role="alert" className="text-sm text-destructive">
            {send.error.message}
          </p>
        )}
        {settings.isError ? (
          <p role="alert" className="text-sm text-destructive">
            Could not check Codex.{" "}
            <Button variant="link" onClick={() => void settings.refetch()}>
              Retry
            </Button>
          </p>
        ) : (
          settings.data &&
          !settings.data.engine.ready && (
            <p role="alert" className="text-sm text-destructive">
              {settings.data.engine.error}{" "}
              <Link to="/settings" className="underline">
                AI settings
              </Link>
            </p>
          )
        )}
      </div>
    </section>
  );
}
function ChatMessage({ job, run }: { job: AgentJob; run?: AgentRun }) {
  const active = isAgentWorking([job]);
  const artifacts = run?.artifacts;
  const files = artifacts?.turnFiles ?? [];
  const patch = artifacts?.turnDiff ?? "";
  const pieces = patch.split(/(?=^diff --git )/m).filter(Boolean);
  return (
    <li className="min-w-0 space-y-3">
      {!job.request?.chat?.sessionId && (
        <p className="text-center text-xs text-muted-foreground">
          New conversation
        </p>
      )}
      <div className="ml-4 rounded-lg bg-muted p-3 sm:ml-12">
        <div className="mb-1 flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
          <span>
            You · {job.request?.intent === "implement" ? "Edit" : "Ask"}
          </span>
          <time>
            {job.createdAt ? new Date(job.createdAt).toLocaleString() : ""}
          </time>
        </div>
        <p className="whitespace-pre-wrap break-words text-sm">
          {job.request?.instruction}
        </p>
      </div>
      <div className="min-w-0 rounded-lg border p-3">
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <span>
            {job.request?.presetName || "Codex"} · {job.request?.model}
          </span>
          <Badge
            variant={job.status === "failed" ? "destructive" : "secondary"}
          >
            {job.status === "completed" ? "Replied" : job.status}
          </Badge>
        </div>
        {run?.output ? (
          <AIMarkdown>{run.output}</AIMarkdown>
        ) : (
          <p className="mt-3 text-sm text-muted-foreground">
            {active ? "Waiting for a response…" : "No response was recorded."}
          </p>
        )}
        {job.error && (
          <p role="alert" className="mt-3 text-sm text-destructive">
            {job.error}
          </p>
        )}
        {artifacts?.warning && (
          <p role="alert" className="mt-3 text-sm text-destructive">
            {artifacts.warning}
          </p>
        )}
        {files.length > 0 && (
          <div className="mt-4 space-y-2 border-t pt-3">
            <h3 className="text-sm font-medium">
              Files changed this turn · {files.length}
            </h3>
            {files.map((file, index) => (
              <details key={file} className="min-w-0 rounded border p-2">
                <summary className="cursor-pointer break-all font-mono text-xs">
                  {file}
                </summary>
                {pieces.length === files.length && (
                  <pre className="mt-2 max-h-80 overflow-auto rounded bg-muted p-3 text-xs">
                    <code>{pieces[index]}</code>
                  </pre>
                )}
              </details>
            ))}
            {pieces.length !== files.length && patch && (
              <details>
                <summary className="cursor-pointer text-sm">Full patch</summary>
                <pre className="mt-2 max-h-80 overflow-auto bg-muted p-3 text-xs">
                  <code>{patch}</code>
                </pre>
              </details>
            )}
          </div>
        )}
      </div>
    </li>
  );
}
