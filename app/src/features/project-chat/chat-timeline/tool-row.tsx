import { useState } from "react";
import { ChevronRight } from "lucide-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { describeTool, formatDuration, type Names } from "@/features/project-chat/activity";
import { ToolIcon } from "@/features/project-chat/chat-timeline/tool-icon";
import type { ChatActivity } from "@/features/project-chat/types";
import { cn } from "@/lib/utils";

const pretty = (value: unknown) => JSON.stringify(value, null, 2);

// One tool call on a single line; it opens to what the tool was given and
// what it returned.
export function ToolRow({ activity, names }: { activity: ChatActivity; names: Names }) {
  const [open, setOpen] = useState(false);
  const step = describeTool(activity, names);
  const live = activity.status === "inProgress";
  const failed = activity.status === "failed";
  const details = !live && (activity.arguments || activity.result || activity.error);

  return (
    <div className="flex flex-col">
      <button
        type="button"
        disabled={!details}
        aria-expanded={details ? open : undefined}
        onClick={() => setOpen((value) => !value)}
        className="group/row flex h-7 w-full min-w-0 items-center gap-2 rounded-md px-1 text-left text-sm text-muted-foreground enabled:hover:bg-accent/40"
      >
        <span className="flex size-5 shrink-0 items-center justify-center">
          <ToolIcon name={step.icon} className={cn("size-4", failed && "text-destructive/70")} />
        </span>
        <Tooltip>
          <TooltipTrigger asChild>
            <span className={cn("min-w-0 truncate", live && "live-shine", failed && "text-destructive/80")}>
              {step.label}
            </span>
          </TooltipTrigger>
          <TooltipContent className="max-w-sm">{step.label}</TooltipContent>
        </Tooltip>
        {!live && activity.durationMs ? (
          <span className="ml-auto shrink-0 text-xs tabular-nums opacity-0 group-hover/row:opacity-100 pointer-coarse:opacity-100">
            {formatDuration(activity.durationMs)}
          </span>
        ) : null}
        {details && (
          <ChevronRight
            className={cn(
              "size-3.5 shrink-0 opacity-0 transition-transform group-hover/row:opacity-100 group-focus-visible/row:opacity-100",
              !activity.durationMs && "ml-auto",
              open && "rotate-90 opacity-100",
            )}
          />
        )}
      </button>
      {open && details && (
        <div className="ms-7 mb-1 flex max-h-64 flex-col gap-2 overflow-auto rounded-md bg-muted/40 px-3 py-2 font-mono text-xs text-muted-foreground">
          {activity.error && <p className="whitespace-pre-wrap text-destructive">{activity.error}</p>}
          {activity.arguments && Object.keys(activity.arguments).length > 0 && (
            <pre className="whitespace-pre-wrap break-words">{pretty(activity.arguments)}</pre>
          )}
          {activity.result !== undefined && activity.result !== null && (
            <pre className="whitespace-pre-wrap break-words border-t border-border/60 pt-2">{pretty(activity.result)}</pre>
          )}
        </div>
      )}
    </div>
  );
}
