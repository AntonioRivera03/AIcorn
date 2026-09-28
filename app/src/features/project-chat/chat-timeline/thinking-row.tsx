import { useState } from "react";
import { Brain, ChevronRight } from "lucide-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { AIMarkdown } from "@/features/ai/ai-markdown";
import { thinkingPreview } from "@/features/project-chat/activity";
import type { ChatActivity } from "@/features/project-chat/types";
import { cn } from "@/lib/utils";

// "Thinking" with a moving highlight while live; afterwards a "Thought" row
// that opens to the reasoning summary.
export function ThinkingRow({ activity }: { activity: ChatActivity }) {
  const [open, setOpen] = useState(false);
  const live = activity.status === "inProgress";
  const preview = thinkingPreview(activity.text);
  const expandable = !live && !!activity.text?.trim();

  return (
    <div className="flex flex-col">
      <button
        type="button"
        disabled={!expandable}
        aria-expanded={expandable ? open : undefined}
        onClick={() => setOpen((value) => !value)}
        className="group/row flex h-7 w-full min-w-0 items-center gap-2 rounded-md px-1 text-left text-sm text-muted-foreground enabled:hover:bg-accent/40"
      >
        <span className="flex size-5 shrink-0 items-center justify-center">
          <Brain className={cn("size-4", !live && "opacity-70")} />
        </span>
        {live ? (
          <span className="live-shine truncate">{preview || "Thinking"}</span>
        ) : (
          <>
            <span className="shrink-0">Thought</span>
            {preview && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <span className="min-w-0 truncate text-muted-foreground/70">{preview}</span>
                </TooltipTrigger>
                <TooltipContent className="max-w-sm">{preview}</TooltipContent>
              </Tooltip>
            )}
            {expandable && (
              <ChevronRight
                className={cn(
                  "ml-auto size-3.5 shrink-0 opacity-0 transition-transform group-hover/row:opacity-100 group-focus-visible/row:opacity-100",
                  open && "rotate-90 opacity-100",
                )}
              />
            )}
          </>
        )}
      </button>
      {open && activity.text && (
        <div className="ms-7 max-h-96 overflow-auto pe-2 text-muted-foreground [&_div]:text-muted-foreground">
          <AIMarkdown>{activity.text}</AIMarkdown>
        </div>
      )}
    </div>
  );
}
