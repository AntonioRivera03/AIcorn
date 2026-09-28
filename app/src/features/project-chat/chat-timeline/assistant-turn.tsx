import { useState } from "react";
import { ChevronRight } from "lucide-react";
import { AIMarkdown } from "@/features/ai/ai-markdown";
import type { TaskOwner } from "@/features/ai/queries/use-task-ownership";
import { formatDuration, taskChanges, type Names } from "@/features/project-chat/activity";
import { CopyButton } from "@/features/project-chat/chat-timeline/copy-button";
import { TaskChanges } from "@/features/project-chat/chat-timeline/task-changes";
import { WorkLog } from "@/features/project-chat/chat-timeline/work-log";
import type { MentionTask } from "@/features/project-chat/mentions";
import { isWorking, serverTime, type ChatTurn } from "@/features/project-chat/types";
import { useNow } from "@/features/project-chat/use-now";
import { cn } from "@/lib/utils";

type AssistantTurnProps = {
  turn: ChatTurn;
  names: Names;
  tasks: MentionTask[];
  owners: Map<number, TaskOwner>;
};

// Chatter's side of a turn, laid out like T3 Code: a "Working for 12s" header
// with its work underneath while live; afterwards the work folds behind
// "Worked for 1m 12s", leaving the answer and what it changed.
export function AssistantTurn({ turn, names, tasks, owners }: AssistantTurnProps) {
  const [open, setOpen] = useState(false);
  const working = isWorking(turn.status);
  const now = useNow(working);
  const started = serverTime(turn.createdAt).getTime();
  const ended = turn.finishedAt ? serverTime(turn.finishedAt).getTime() : now;
  const elapsed = formatDuration(Math.max(0, ended - started));
  const activity = turn.activity ?? [];
  const changes = working ? [] : taskChanges(activity, names);

  const header = working
    ? turn.status === "pending"
      ? "Waiting to start"
      : turn.status === "canceling"
        ? "Stopping…"
        : `Working for ${elapsed}`
    : turn.status === "canceled"
      ? `You stopped after ${elapsed}`
      : turn.status === "completed"
        ? `Worked for ${elapsed}`
        : `Stopped after ${elapsed}`;
  const showWork = working || open;
  const nothingYet = working && activity.length === 0 && !turn.output;

  return (
    <div className="group/message flex flex-col">
      <h3 className="sr-only">Chatter</h3>
      <button
        type="button"
        disabled={working || activity.length === 0}
        aria-expanded={!working && activity.length > 0 ? open : undefined}
        onClick={() => setOpen((value) => !value)}
        className="group/header mb-1.5 flex items-center gap-1 border-b border-border/60 pb-1.5 text-left text-sm tabular-nums text-muted-foreground enabled:hover:text-foreground"
      >
        {header}
        {!working && activity.length > 0 && (
          <ChevronRight className={cn("size-3.5 transition-transform", open && "rotate-90")} />
        )}
      </button>

      {showWork && activity.length > 0 && (
        <div className="mb-2">
          <WorkLog activity={activity} names={names} live={working} />
        </div>
      )}
      {nothingYet && (
        <p className="live-shine px-1 text-sm">{turn.status === "pending" ? "Queued" : "Thinking"}</p>
      )}

      {turn.output && (
        <div className="px-1 [&>div]:text-foreground/85 [&>div]:leading-relaxed">
          <AIMarkdown linkTasks={tasks} owners={owners}>
            {turn.output}
          </AIMarkdown>
        </div>
      )}
      {turn.error && turn.status !== "canceled" && (
        <p role="alert" className="px-1 text-sm text-destructive">
          {turn.error}
        </p>
      )}

      <TaskChanges changes={changes} />

      {!working && turn.output && (
        <div className="mt-1 flex items-center gap-1 text-xs text-muted-foreground opacity-0 transition-opacity group-hover/message:opacity-100 focus-within:opacity-100 pointer-coarse:opacity-100">
          <CopyButton text={turn.output} label="Copy answer" />
          {turn.finishedAt && (
            <time dateTime={serverTime(turn.finishedAt).toISOString()}>
              {serverTime(turn.finishedAt).toLocaleTimeString(undefined, {
                hour: "numeric",
                minute: "2-digit",
              })}
            </time>
          )}
        </div>
      )}
    </div>
  );
}
