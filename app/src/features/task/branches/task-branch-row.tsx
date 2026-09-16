import { useRef, useState } from "react";
import { BranchPreviews } from "@/features/environments/branch-previews";
import { Check, Copy, GitBranch, GitMerge, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { isAgentWorking } from "@/features/agentJob/queries/useAgentJobs";
import { MergeBranchDialog } from "@/features/task/branches/merge-branch-dialog";
import type { TaskBranch } from "@/features/task/branches/types";

export function TaskBranchRow({ branch }: { branch: TaskBranch }) {
  const [reviewOpen, setReviewOpen] = useState(false);
  const reviewButton = useRef<HTMLButtonElement>(null);
  const active = isAgentWorking([branch]);
  const merged = branch.mergedInto.length > 0 && !branch.dirty;
  const copyBranch = async () => {
    try {
      await navigator.clipboard.writeText(branch.branch);
      toast.success("Branch name copied");
    } catch {
      toast.error("Could not copy branch name");
    }
  };
  return (
    <>
      <div className="space-y-2 p-3 sm:p-4">
        <div className="flex items-start gap-2.5">
          <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <GitBranch className="size-4" />
          </span>
          <div className="min-w-0 flex-1">
            <div className="flex items-start gap-1">
              <code className="break-all pt-1 text-xs sm:text-sm">
                {branch.branch}
              </code>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-7 shrink-0 text-muted-foreground"
                    aria-label={`Copy branch ${branch.branch}`}
                    onClick={() => void copyBranch()}
                  >
                    <Copy className="size-3.5" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Copy branch name</TooltipContent>
              </Tooltip>
            </div>
            <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <span className="capitalize">
                {branch.intent || "Agent"} · Run #{branch.jobId}
              </span>
              <span aria-hidden="true">·</span>
              <time dateTime={branch.createdAt}>
                {new Date(branch.createdAt).toLocaleDateString(undefined, {
                  month: "short",
                  day: "numeric",
                })}
              </time>
              <Badge variant="secondary" className="gap-1 text-xs font-normal">
                {active ? (
                  <>
                    <Loader2 className="size-3 animate-spin motion-reduce:animate-none" />
                    Agent working
                  </>
                ) : branch.problem ? (
                  "Unavailable"
                ) : merged ? (
                  <>
                    <Check className="size-3" />
                    Merged
                  </>
                ) : branch.dirty ? (
                  "Uncommitted edits"
                ) : (
                  "Ready to review"
                )}
              </Badge>
              {!active && branch.status !== "completed" && (
                <span>
                  {branch.status === "failed"
                    ? "Run failed · partial work"
                    : `${branch.status} run`}
                </span>
              )}
            </div>
          </div>
        </div>
        <div className="flex flex-wrap items-center justify-between gap-2 pl-10.5">
          {merged && (
            <p className="break-all text-xs text-muted-foreground">
              Included in{" "}
              <span className="font-mono">{branch.mergedInto.join(", ")}</span>
            </p>
          )}
          {branch.problem ? (
            <p className="break-all text-xs text-destructive">
              {branch.problem}
            </p>
          ) : (
            <Button
              size="sm"
              variant="outline"
              className="ml-auto h-7 gap-1.5 text-xs"
              ref={reviewButton}
              disabled={active}
              onClick={() => setReviewOpen(true)}
            >
              <GitMerge className="size-3.5" />
              Review & merge
            </Button>
          )}
        </div>
        <details className="pl-10.5 text-xs text-muted-foreground">
          <summary className="w-fit cursor-pointer">Location</summary>
          <dl className="mt-2 space-y-1">
            <dt>Repository</dt>
            <dd className="select-all break-all font-mono">
              {branch.repoPath}
            </dd>
            {branch.workspace && (
              <>
                <dt className="pt-1">Agent workspace</dt>
                <dd className="select-all break-all font-mono">
                  {branch.workspace}
                </dd>
              </>
            )}
          </dl>
        </details>
        <BranchPreviews taskId={branch.taskId} jobId={branch.jobId} active={active} />
      </div>
      {reviewOpen && (
        <MergeBranchDialog
          branch={branch}
          onClose={() => setReviewOpen(false)}
          onReturnFocus={() => reviewButton.current?.focus()}
        />
      )}
    </>
  );
}
