import { Badge } from "@/components/ui/badge";
import { Bot } from "lucide-react";
import { useContext, useMemo } from "react";
import { ProjectContext } from "@/contexts/project/ProjectContext";
import { useActiveAgentJobs } from "@/features/agentJob/queries/useActiveAgentJobs";

type Props = {
  taskId: number;
  className?: string;
  compact?: boolean;
};

export function AgentWorkingBadge({ taskId, className, compact = false }: Props) {
  const { Project } = useContext(ProjectContext);
  const projectId = Project?.ID ?? 0;
  const { activeTaskIds } = useActiveAgentJobs(projectId);
  const working = useMemo(() => activeTaskIds.has(taskId), [activeTaskIds, taskId]);
  if (!working) return null;

  if (compact) {
    return (
      <span
        aria-label="Agent working"
        className={
          "inline-flex items-center gap-1 shrink-0 max-w-full " +
          (className ?? "")
        }
      >
        <span className="relative flex size-2 shrink-0">
          <span className="absolute inline-flex h-full w-full rounded-full bg-primary opacity-75 animate-ping [animation-duration:2s] motion-reduce:animate-none" />
          <span className="relative inline-flex rounded-full size-2 bg-primary animate-pulse [animation-duration:2s] motion-reduce:animate-none" />
        </span>
        <Bot className="size-3 animate-spin [animation-duration:1s] text-primary shrink-0 motion-reduce:animate-none" aria-hidden="true" />
        <span className="text-xs font-medium text-primary truncate">Working</span>
      </span>
    );
  }

  return (
    <Badge
      variant="default"
      aria-label="Agent working"
      className={
        "bg-primary text-primary-foreground border-border gap-1 max-w-full " +
        (className ?? "")
      }
    >
      <span className="relative flex size-2 shrink-0" aria-hidden="true">
        <span className="absolute inline-flex h-full w-full rounded-full bg-primary-foreground opacity-60 animate-ping [animation-duration:2s] motion-reduce:animate-none" />
        <span className="relative inline-flex rounded-full size-2 bg-primary-foreground animate-pulse [animation-duration:2s] motion-reduce:animate-none" />
      </span>
      <Bot className="size-3 animate-spin [animation-duration:1s] shrink-0 motion-reduce:animate-none" aria-hidden="true" />
      <span className="truncate">Working</span>
    </Badge>
  );
}
