import { Link } from "@tanstack/react-router";
import type { TaskOwner } from "@/features/ai/queries/use-task-ownership";
import { CopyButton } from "@/features/project-chat/chat-timeline/copy-button";
import { splitMentionSegments, type MentionTask } from "@/features/project-chat/mentions";
import { serverTime, type ChatTurn } from "@/features/project-chat/types";

type UserMessageProps = {
  turn: ChatTurn;
  tasks: MentionTask[];
  owners: Map<number, TaskOwner>;
};

export function UserMessage({ turn, tasks, owners }: UserMessageProps) {
  const sent = serverTime(turn.createdAt);
  const segments = splitMentionSegments(turn.message, tasks);
  return (
    <div className="group/message flex flex-col items-end gap-1">
      <h3 className="sr-only">You</h3>
      <div className="max-w-[80%] whitespace-pre-wrap break-words rounded-2xl bg-muted px-4 py-2.5 text-sm">
        {segments.map((segment, index) =>
          segment.type === "text" ? (
            segment.text
          ) : (
            <span key={index}>
              <Link
                to="/app/task/$taskId"
                params={{ taskId: String(segment.id) }}
                className="text-primary underline-offset-4 hover:underline"
              >
                {segment.text}
              </Link>
              {owners.get(segment.id) && (
                <span className="text-xs text-muted-foreground">
                  {" "}
                  ({owners.get(segment.id)!.name} · {owners.get(segment.id)!.state})
                </span>
              )}
            </span>
          ),
        )}
      </div>
      <div className="flex items-center gap-1 text-xs text-muted-foreground opacity-0 transition-opacity group-hover/message:opacity-100 focus-within:opacity-100 pointer-coarse:opacity-100">
        <time dateTime={sent.toISOString()}>
          {sent.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" })}
        </time>
        <CopyButton text={turn.message} label="Copy message" />
      </div>
    </div>
  );
}
