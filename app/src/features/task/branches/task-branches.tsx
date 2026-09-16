import { GitBranch, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { TaskBranchRow } from "@/features/task/branches/task-branch-row";
import { useTaskBranches } from "@/features/task/branches/queries/use-task-branches";

export function TaskBranches({ taskId }: { taskId: number }) {
  const branches = useTaskBranches(taskId);
  if (!taskId || (!branches.error && !branches.data?.length)) return null;
  const [latest, ...earlier] = branches.data || [];
  return (
    <section
      aria-label="Code branches"
      className="my-2 rounded-xl border border-border bg-card"
    >
      <div className="flex items-center gap-2 border-b border-border px-3 py-2 sm:px-4">
        <GitBranch className="size-4 text-muted-foreground" />
        <h2 className="text-sm font-medium">Code branches</h2>
        {!!branches.data?.length && (
          <span className="text-xs text-muted-foreground">
            {branches.data.length}
          </span>
        )}
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              aria-label="Refresh code branches"
              variant="ghost"
              size="icon"
              className="ml-auto size-7 text-muted-foreground"
              disabled={branches.isFetching}
              onClick={() => void branches.refetch()}
            >
              <RefreshCw
                className={`size-3.5 ${branches.isFetching ? "animate-spin motion-reduce:animate-none" : ""}`}
              />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Refresh from Git</TooltipContent>
        </Tooltip>
      </div>
      {branches.error && (
        <p role="alert" className="p-3 text-sm text-destructive">
          Could not read branches. {branches.error.message}
        </p>
      )}
      {latest && <TaskBranchRow key={latest.jobId} branch={latest} />}
      {!!earlier.length && (
        <details className="border-t border-border">
          <summary className="cursor-pointer px-4 py-2 text-xs text-muted-foreground">
            {earlier.length} earlier{" "}
            {earlier.length === 1 ? "branch" : "branches"}
          </summary>
          <div className="divide-y divide-border">
            {earlier.map((branch) => (
              <TaskBranchRow key={branch.jobId} branch={branch} />
            ))}
          </div>
        </details>
      )}
    </section>
  );
}
