import { useState } from "react";
import { ChevronRight, Wrench } from "lucide-react";
import { summarizeTools, type Names } from "@/features/project-chat/activity";
import { ToolRow } from "@/features/project-chat/chat-timeline/tool-row";
import type { ChatActivity } from "@/features/project-chat/types";
import { cn } from "@/lib/utils";

// Consecutive tool calls folded into one sentence, e.g. "Read 3 tasks and
// created 1 task", that opens to the calls themselves.
export function ToolGroup({ activity, names }: { activity: ChatActivity[]; names: Names }) {
  const [open, setOpen] = useState(false);
  const failed = activity.some((a) => a.status === "failed");

  return (
    <div className="flex flex-col">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
        className="group/row flex h-7 w-full min-w-0 items-center gap-2 rounded-md px-1 text-left text-sm text-muted-foreground hover:bg-accent/40"
      >
        <span className="flex size-5 shrink-0 items-center justify-center">
          <Wrench className={cn("size-4", failed && "text-destructive/70")} aria-hidden />
        </span>
        <span className="min-w-0 flex-1">
          {summarizeTools(activity)}
          {failed && <span className="text-destructive/80"> · some failed</span>}
        </span>
        <ChevronRight
          className={cn(
            "size-3.5 shrink-0 opacity-0 transition-transform group-hover/row:opacity-100 group-focus-visible/row:opacity-100",
            open && "rotate-90 opacity-100",
          )}
        />
      </button>
      {open && (
        <div className="ms-3 flex max-h-[min(18rem,50dvh)] flex-col overflow-auto border-l border-border/60 ps-2">
          {activity.map((a) => (
            <ToolRow key={a.id} activity={a} names={names} />
          ))}
        </div>
      )}
    </div>
  );
}
