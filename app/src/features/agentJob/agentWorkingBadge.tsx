import { useContext } from "react";
import { Sparkles, Loader2 } from "lucide-react";
import { ProjectContext } from "@/contexts/project/ProjectContext";
import { useActiveAgentJobs } from "@/features/agentJob/queries/useActiveAgentJobs";
import { useAI } from "@/features/ai/ai-context";
import { cn } from "@/lib/utils";

type Props = { taskId: number; className?: string; compact?: boolean };

export function AgentWorkingBadge({
  taskId,
  className,
  compact = false,
}: Props) {
  const { Project, Tasks } = useContext(ProjectContext);
  const { data: jobs } = useActiveAgentJobs(Project?.ID ?? 0);
  const { openAI } = useAI();
  const job = jobs?.find((job) => job.task === taskId);
  if (!job) return null;
  const labels: Record<string, string> = {
    pending: "Queued",
    claimed: "Starting",
    running: "Running",
    canceling: "Stopping",
    completed: "Result ready",
    failed: "AI failed",
    canceled: "Stopped",
    interrupted: "Interrupted",
  };
  const running = ["claimed", "running", "canceling"].includes(job.status);
  return (
    <button
      type="button"
      className={cn(
        "inline-flex items-center gap-1.5 rounded-md px-1.5 py-0.5 text-xs text-muted-foreground hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        !compact && "border border-border",
        job.status === "failed" && "text-destructive",
        className,
      )}
      aria-label={`AI: ${labels[job.status] || job.status}. Open results`}
      onClick={(e) => {
        e.stopPropagation();
        openAI({
          id: taskId,
          name:
            Tasks.find((task) => task.ID === taskId)?.Name || `Task ${taskId}`,
        });
      }}
    >
      {running ? (
        <Loader2 className="size-3 animate-spin motion-reduce:animate-none" />
      ) : (
        <Sparkles className="size-3" />
      )}
      {labels[job.status] || job.status}
    </button>
  );
}
