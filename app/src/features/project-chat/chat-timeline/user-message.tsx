import { CopyButton } from "@/features/project-chat/chat-timeline/copy-button";
import { serverTime, type ChatTurn } from "@/features/project-chat/types";

export function UserMessage({ turn }: { turn: ChatTurn }) {
  const sent = serverTime(turn.createdAt);
  return (
    <div className="group/message flex flex-col items-end gap-1">
      <h3 className="sr-only">You</h3>
      <div className="max-w-[80%] whitespace-pre-wrap break-words rounded-2xl bg-muted px-4 py-2.5 text-sm">
        {turn.message}
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
