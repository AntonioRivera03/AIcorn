import { useEffect, useId, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Hash, MessageSquare, Send, Square } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { AIMarkdown } from "@/features/ai/ai-markdown";
import { useAISettings } from "@/features/ai/queries/use-ai";
import { useTaskOwnership } from "@/features/ai/queries/use-task-ownership";
import { cn } from "@/lib/utils";
import {
  filterMentions,
  insertMention,
  mentionAt,
  referencedTasks,
  type MentionTask,
} from "./mentions";

type Turn = {
  id: number;
  message: string;
  status: string;
  output: string;
  progress: string;
  error: string;
  createdAt: string;
};
type Conversation = { id: number; projectId: number; turns: Turn[] };
const working = (status: string) =>
  ["pending", "running", "canceling"].includes(status);
async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const r = await fetch(url, init);
  if (!r.ok) throw new Error((await r.text()).trim());
  return r.status === 204 ? (undefined as T) : r.json();
}
function initialRequestKey(
  key: string,
): { message: string; key: string } | null {
  try {
    const value = JSON.parse(localStorage.getItem(`${key}-request`) ?? "null");
    return typeof value?.message === "string" && typeof value?.key === "string"
      ? value
      : null;
  } catch {
    return null;
  }
}
function initialDraft(key: string) {
  try {
    return localStorage.getItem(key) ?? "";
  } catch {
    return "";
  }
}
export function ProjectChat({ projectId }: { projectId: number }) {
  const base = `/api/project-chats/project/${projectId}`;
  const key = ["project-chat", projectId];
  const client = useQueryClient();
  const settings = useAISettings();
  const ownership = useTaskOwnership(projectId);
  const history = useQuery({
    queryKey: key,
    queryFn: () => request<Conversation>(base),
    refetchInterval: 1000,
  });
  const tasks = useQuery({
    queryKey: ["project-chat-tasks", projectId],
    queryFn: () => request<MentionTask[]>(`${base}/tasks`),
    refetchInterval: 3000,
  });
  const draftKey = `aycorn-project-chat-draft-${projectId}`;
  const [message, setMessage] = useState(() => initialDraft(draftKey));
  const [caret, setCaret] = useState(message.length);
  const [choice, setChoice] = useState(0);
  const [dismissed, setDismissed] = useState(false);
  const composer = useRef<HTMLTextAreaElement>(null);
  const timeline = useRef<HTMLDivElement>(null);
  const follow = useRef(true);
  const pickerId = useId();
  const requestKey = useRef<{ message: string; key: string } | null>(
    initialRequestKey(draftKey),
  );
  const turns = history.data?.turns ?? [];
  const active = turns.find((t) => working(t.status));
  const mention = mentionAt(message, caret);
  const options = mention
    ? filterMentions(tasks.data ?? [], mention).slice(0, 20)
    : [];
  const pickerOpen = !!mention && !dismissed;
  const selected = Math.min(choice, Math.max(0, options.length - 1));
  const selectedTaskID = options[selected]?.id;
  useEffect(() => {
    if (pickerOpen && selectedTaskID)
      document
        .getElementById(`${pickerId}-${selectedTaskID}`)
        ?.scrollIntoView({ block: "nearest" });
  }, [pickerOpen, pickerId, selectedTaskID]);
  const lastStatus = turns.at(-1)?.status;
  const revision = turns
    .map((t) => `${t.id}:${t.status}:${t.output.length}`)
    .join("|");
  useEffect(() => {
    try {
      if (message) localStorage.setItem(draftKey, message);
      else localStorage.removeItem(draftKey);
    } catch {
      /* Draft remains editable if storage is unavailable. */
    }
  }, [draftKey, message]);
  useEffect(() => {
    if (follow.current && timeline.current)
      timeline.current.scrollTop = timeline.current.scrollHeight;
  }, [revision]);
  useEffect(() => {
    void client.invalidateQueries({ queryKey: ["projectDetails", projectId] });
    void client.invalidateQueries({ queryKey: ["task-ownership", projectId] });
    void client.invalidateQueries({
      queryKey: ["project-chat-tasks", projectId],
    });
  }, [client, projectId, lastStatus]);
  const send = useMutation({
    mutationFn: (input: { message: string; key: string; taskIds: number[] }) =>
      request<Turn>(`${base}/messages`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(input),
      }),
    onSuccess: (turn, input) => {
      requestKey.current = null;
      try {
        localStorage.removeItem(`${draftKey}-request`);
      } catch {
        /* Retry keys remain usable in memory. */
      }
      setMessage((old) => (old === input.message ? "" : old));
      setDismissed(true);
      follow.current = true;
      client.setQueryData<Conversation>(key, (old) =>
        old
          ? {
              ...old,
              turns: [...old.turns.filter((t) => t.id !== turn.id), turn],
            }
          : old,
      );
      void client.invalidateQueries({ queryKey: key });
      composer.current?.focus();
    },
  });
  const cancel = useMutation({
    mutationFn: (id: number) =>
      request<void>(`${base}/${id}/cancel`, { method: "POST" }),
    onSuccess: () => void client.invalidateQueries({ queryKey: key }),
  });
  const blocked =
    !!active ||
    send.isPending ||
    !message.trim() ||
    history.isPending ||
    history.isError ||
    !settings.data?.engine.ready;
  function submit() {
    if (blocked) return;
    if (requestKey.current?.message !== message)
      requestKey.current = { message, key: crypto.randomUUID() };
    try {
      localStorage.setItem(
        `${draftKey}-request`,
        JSON.stringify(requestKey.current),
      );
    } catch {
      /* A retry within this view still reuses its key. */
    }
    send.mutate({
      message,
      key: requestKey.current.key,
      taskIds: referencedTasks(message, tasks.data ?? []).map((t) => t.id),
    });
  }
  function choose(task: MentionTask) {
    if (!mention) return;
    const next = insertMention(message, mention, task.id);
    setMessage(next.text);
    setCaret(next.caret);
    setDismissed(true);
    requestAnimationFrame(() => {
      composer.current?.focus();
      composer.current?.setSelectionRange(next.caret, next.caret);
    });
  }
  return (
    <section
      aria-label="Project chat"
      className="flex h-[calc(100vh-15rem)] min-h-[32rem] flex-col overflow-hidden rounded-xl border bg-card"
    >
      <header className="flex items-center justify-between gap-3 border-b px-5 py-4">
        <div>
          <h2 className="flex items-center gap-2 font-semibold">
            <MessageSquare className="size-4" />
            Chatter
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            Your project conversation · tasks, plans, and project knowledge
          </p>
        </div>
        <span className="text-xs text-muted-foreground">
          {active ? "Working…" : "Persistent conversation"}
        </span>
      </header>
      <div
        ref={timeline}
        role="log"
        aria-label="Conversation"
        className="min-h-0 flex-1 space-y-6 overflow-y-auto p-4 sm:p-6"
        onScroll={(e) => {
          const el = e.currentTarget;
          follow.current =
            el.scrollHeight - el.scrollTop - el.clientHeight < 100;
        }}
      >
        {history.isPending ? (
          <p role="status">Loading conversation…</p>
        ) : history.isError ? (
          <p role="alert">
            {history.error.message}{" "}
            <Button variant="link" onClick={() => void history.refetch()}>
              Retry
            </Button>
          </p>
        ) : turns.length === 0 ? (
          <div className="mx-auto flex max-w-lg flex-col gap-3 py-10 text-center text-muted-foreground">
            <MessageSquare className="mx-auto size-8" />
            <h3 className="font-medium text-foreground">
              Think it through. Put it in motion.
            </h3>
            <p className="text-sm">
              Ask about your project, turn an idea into tasks, or send a task to
              an agent. Chatter follows your workflow and respects tasks another
              agent owns.
            </p>
            <p className="text-sm">
              Try “Create a task for Fabiana’s request” or “Help me plan #123.”
            </p>
          </div>
        ) : (
          turns.map((turn) => (
            <article key={turn.id} className="mx-auto max-w-4xl space-y-4">
              <div className="ml-auto max-w-[90%] rounded-xl bg-muted p-4">
                <p className="mb-2 text-xs font-medium text-muted-foreground">
                  You
                </p>
                <p className="whitespace-pre-wrap break-words text-sm">
                  {turn.message}
                </p>
                <References text={turn.message} tasks={tasks.data ?? []} />
              </div>
              <div className="max-w-full space-y-2">
                <p className="text-xs font-medium text-muted-foreground">
                  Chatter
                </p>
                {turn.output ? (
                  <AIMarkdown>{turn.output}</AIMarkdown>
                ) : working(turn.status) ? (
                  <p role="status" className="text-sm text-muted-foreground">
                    {turn.progress || "Queued…"}
                  </p>
                ) : null}
                <References text={turn.output} tasks={tasks.data ?? []} />
                {turn.error && (
                  <p role="alert" className="text-sm text-destructive">
                    {turn.error}
                  </p>
                )}
                {!working(turn.status) && (
                  <p className="text-xs text-muted-foreground">
                    {turn.status} ·{" "}
                    {new Date(
                      turn.createdAt.includes("T")
                        ? turn.createdAt
                        : turn.createdAt.replace(" ", "T") + "Z",
                    ).toLocaleString()}
                  </p>
                )}
              </div>
            </article>
          ))
        )}
      </div>
      <div className="border-t bg-background p-4">
        {active && (
          <div className="mb-3 flex items-center justify-between gap-3 text-sm">
            <span role="status">
              {active.progress ||
                (active.status === "canceling" ? "Stopping…" : "Queued…")}
            </span>
            <Button
              variant="outline"
              size="sm"
              disabled={cancel.isPending || active.status === "canceling"}
              onClick={() => cancel.mutate(active.id)}
            >
              <Square className="size-3" />
              Stop
            </Button>
          </div>
        )}
        <div className="relative mx-auto max-w-4xl">
          {pickerOpen && (
            <div
              id={pickerId}
              role="listbox"
              aria-label="Project tasks"
              className="absolute right-0 bottom-full left-0 z-30 mb-2 max-h-64 overflow-y-auto rounded-lg border bg-popover p-1 shadow-lg"
            >
              <p className="px-3 py-2 text-xs text-muted-foreground">
                {mention?.mode === "title"
                  ? "Search titles"
                  : "Search task numbers"}{" "}
                · ↑↓ to choose · Enter to insert
              </p>
              {options.map((task, index) => {
                const owner = ownership.data?.find((o) => o.taskId === task.id);
                return (
                  <button
                    key={task.id}
                    id={`${pickerId}-${task.id}`}
                    type="button"
                    role="option"
                    aria-selected={index === selected}
                    className={cn(
                      "flex w-full items-center gap-2 rounded px-3 py-2 text-left text-sm",
                      index === selected && "bg-accent text-accent-foreground",
                    )}
                    onMouseDown={(e) => e.preventDefault()}
                    onClick={() => choose(task)}
                  >
                    <span className="font-mono text-xs text-muted-foreground">
                      #{task.id}
                    </span>
                    <span className="min-w-0 flex-1 truncate">
                      {task.title || "Untitled task"}
                    </span>
                    {owner && (
                      <span className="text-xs text-muted-foreground">
                        {owner.name}
                      </span>
                    )}
                  </button>
                );
              })}
              {!options.length && (
                <p className="p-3 text-sm text-muted-foreground">
                  {tasks.isPending
                    ? "Loading tasks…"
                    : tasks.isError
                      ? "Could not load tasks."
                      : "No matching tasks"}
                </p>
              )}
            </div>
          )}
          <Textarea
            ref={composer}
            aria-label="Message Chatter"
            role="combobox"
            aria-autocomplete="list"
            aria-expanded={pickerOpen}
            aria-controls={pickerOpen ? pickerId : undefined}
            aria-activedescendant={
              pickerOpen && options[selected]
                ? `${pickerId}-${selectedTaskID}`
                : undefined
            }
            value={message}
            maxLength={32000}
            className="min-h-24 resize-y"
            placeholder={
              'Ask Chatter… Type # for tasks, or #" to search titles.'
            }
            onChange={(e) => {
              setMessage(e.target.value);
              setCaret(e.target.selectionStart);
              setChoice(0);
              setDismissed(false);
            }}
            onSelect={(e) => setCaret(e.currentTarget.selectionStart)}
            onKeyDown={(e) => {
              if (e.nativeEvent.isComposing) return;
              if (pickerOpen && e.key === "Escape") {
                e.preventDefault();
                setDismissed(true);
                return;
              }
              if (pickerOpen && options.length) {
                if (e.key === "ArrowDown" || e.key === "ArrowUp") {
                  e.preventDefault();
                  setChoice(
                    (selected +
                      (e.key === "ArrowDown" ? 1 : options.length - 1)) %
                      options.length,
                  );
                  return;
                }
                if (e.key === "Enter" || e.key === "Tab") {
                  e.preventDefault();
                  choose(options[selected]);
                  return;
                }
              }
              if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
                e.preventDefault();
                submit();
              }
            }}
          />
          <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                const start =
                  composer.current?.selectionStart ?? message.length;
                const before = message.slice(0, start);
                const insert = (before && !/\s$/.test(before) ? " " : "") + "#";
                setMessage(before + insert + message.slice(start));
                setCaret(start + insert.length);
                setDismissed(false);
                setChoice(0);
                requestAnimationFrame(() => {
                  composer.current?.focus();
                  composer.current?.setSelectionRange(
                    start + insert.length,
                    start + insert.length,
                  );
                });
              }}
            >
              <Hash className="size-4" />
              Reference a task
            </Button>
            <div className="flex items-center gap-3">
              <span className="text-xs text-muted-foreground">
                Ctrl/⌘ + Enter
              </span>
              <Button onClick={submit} disabled={blocked}>
                <Send className="size-4" />
                {send.isPending ? "Sending…" : "Send"}
              </Button>
            </div>
          </div>
          {(send.error || cancel.error) && (
            <p role="alert" className="mt-2 text-sm text-destructive">
              {send.error?.message || cancel.error?.message}
            </p>
          )}
          {settings.isError ? (
            <p role="alert" className="mt-2 text-sm text-destructive">
              Could not check AI settings.
            </p>
          ) : settings.data && !settings.data.engine.ready ? (
            <p className="mt-2 text-sm text-muted-foreground">
              {settings.data.engine.error}{" "}
              <Link to="/settings" className="underline">
                AI settings
              </Link>
            </p>
          ) : null}
        </div>
      </div>
    </section>
  );
}
function References({ text, tasks }: { text: string; tasks: MentionTask[] }) {
  const refs = referencedTasks(text, tasks);
  if (!refs.length) return null;
  return (
    <div className="mt-2 flex flex-wrap gap-2">
      {refs.map((task) => (
        <Link
          key={task.id}
          to="/task/$taskId"
          params={{ taskId: String(task.id) }}
          className="max-w-full truncate rounded border px-2 py-1 text-xs hover:bg-accent"
        >
          #{task.id} · {task.title || "Untitled task"}
        </Link>
      ))}
    </div>
  );
}
