import { useEffect, useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { useAgentJobs } from "@/queries/useAgentJobs";
import type { AgentRun } from "@/features/agentJob/queries/useAgentJobs";
import { toast } from "sonner";
import { CheckCircle2, XCircle, Clock3, Copy, GitBranch, ChevronDown } from "lucide-react";
import { cn } from "@/lib/utils";

type Props = {
  taskId: number;
  enabled: boolean;
};

function extractDiff(usageJson: string): string | null {
  if (!usageJson) return null;
  try {
    const parsed = JSON.parse(usageJson) as Record<string, unknown>;
    const candidates = ["diff", "Diff", "gitDiff", "patch", "git_diff"];
    for (const key of candidates) {
      const val = parsed[key];
      if (typeof val === "string" && val.trim().length > 0) return val;
    }
    return null;
  } catch {
    return null;
  }
}

function extractBranch(usageJson: string, taskId: number): string {
  if (usageJson) {
    try {
      const parsed = JSON.parse(usageJson) as Record<string, unknown>;
      const candidates = ["branch", "BranchName", "branchName", "gitBranch"];
      for (const key of candidates) {
        const val = parsed[key];
        if (typeof val === "string" && val.trim().length > 0) return val;
      }
    } catch {
      // ignore parse error
    }
  }
  return `aycorn/task-${taskId}`;
}

function formatTime(iso: string | null): string {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

function RunCard({
  run,
  taskId,
}: {
  run: AgentRun;
  taskId: number;
}) {
  const diff = extractDiff(run.usageJson);
  const branch = extractBranch(run.usageJson, taskId);

  const status: "success" | "failed" | "pending" =
    run.exitCode === null || run.exitCode === undefined
      ? "pending"
      : run.exitCode === 0
        ? "success"
        : "failed";

  const handleCopy = async (text: string, label: string) => {
    try {
      await navigator.clipboard.writeText(text);
      toast(`${label} copied`);
    } catch {
      toast.error(`Failed to copy ${label.toLowerCase()}`);
    }
  };

  const prettyUsage = useMemo(() => {
    if (!run.usageJson) return null;
    try {
      const parsed = JSON.parse(run.usageJson);
      return JSON.stringify(parsed, null, 2);
    } catch {
      return run.usageJson;
    }
  }, [run.usageJson]);

  return (
    <div className="rounded-lg border border-border bg-card p-3 flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        {status === "success" ? (
          <Badge variant="secondary" className="gap-1">
            <CheckCircle2 className="size-3" />
            success
          </Badge>
        ) : status === "failed" ? (
          <Badge variant="destructive" className="gap-1">
            <XCircle className="size-3" />
            failed
          </Badge>
        ) : (
          <Badge variant="outline" className="gap-1">
            <Clock3 className="size-3" />
            pending
          </Badge>
        )}
        {run.exitCode !== null && run.exitCode !== undefined && (
          <span className="text-xs text-muted-foreground">
            exit {run.exitCode}
          </span>
        )}
        {run.summary && (
          <span className="text-sm text-foreground truncate flex-1 min-w-0">
            {run.summary}
          </span>
        )}
        <span className="text-xs text-muted-foreground ml-auto">
          {formatTime(run.createdAt)}
        </span>
      </div>

      <div className="flex items-center gap-2 text-xs">
        <Badge variant="outline" className="gap-1 font-mono">
          <GitBranch className="size-3" />
          {branch}
        </Badge>
        <Button
          variant="ghost"
          size="icon-sm"
          className="size-6"
          onClick={() => handleCopy(branch, "Branch")}
          aria-label="Copy branch name"
        >
          <Copy className="size-3" />
        </Button>
        <span className="text-muted-foreground">
          Leave branch for manual merge
        </span>
      </div>

      <Separator />

      <div className="flex flex-col gap-1.5">
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium text-muted-foreground">Output</span>
          {run.output && (
            <Button
              variant="outline"
              size="sm"
              className="h-7 text-xs"
              onClick={() => handleCopy(run.output, "Output")}
            >
              <Copy className="size-3" />
              Copy
            </Button>
          )}
        </div>
        {run.output ? (
          <div className="prose prose-sm max-w-none dark:prose-invert bg-muted/40 rounded p-3 text-sm leading-relaxed whitespace-pre-wrap break-words border border-border">
            {run.output}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground italic">No output</p>
        )}
      </div>

      <div className="flex flex-col gap-1.5">
        <div className="flex items-center justify-between">
          <span className="text-xs font-medium text-muted-foreground">Diff</span>
          {diff && (
            <Button
              variant="outline"
              size="sm"
              className="h-7 text-xs"
              onClick={() => handleCopy(diff, "Diff")}
            >
              <Copy className="size-3" />
              Copy
            </Button>
          )}
        </div>
        {diff ? (
          <pre className="bg-muted p-3 rounded text-xs overflow-auto max-h-72 whitespace-pre-wrap break-words border border-border font-mono">
            {diff}
          </pre>
        ) : (
          <div className="bg-muted p-3 rounded text-xs border border-border font-mono text-muted-foreground">
            No diff captured yet.
            <span className="block mt-1">
              Leave branch <span className="font-semibold">{branch}</span> for manual merge
              when ready — no auto-merge.
            </span>
          </div>
        )}
        {!diff && run.output && (
          <p className="text-xs text-muted-foreground">
            Leave branch for manual merge — no auto-merge.
          </p>
        )}
      </div>

      {prettyUsage && (
        <details className="rounded border border-border bg-muted/30 p-2">
          <summary className="cursor-pointer text-xs font-medium text-muted-foreground list-inside">
            Usage JSON
          </summary>
          <pre className="mt-2 bg-muted p-3 rounded text-xs overflow-auto max-h-64 whitespace-pre-wrap break-words border border-border font-mono">
            {prettyUsage}
          </pre>
        </details>
      )}
    </div>
  );
}

export function TaskAgentRuns({ taskId, enabled }: Props) {
  const { data, isPending, isFetching, isError, error } = useAgentJobs(taskId, enabled);
  const [sectionOpen, setSectionOpen] = useState(true);

  useEffect(() => {
    if (isError) {
      const msg = error instanceof Error ? error.message : "Failed to load agent runs";
      toast.error(msg);
    }
  }, [isError, error]);

  const runs = useMemo(() => {
    if (!data?.runs) return [];
    return [...data.runs].sort((a, b) => {
      const ta = a.createdAt ? new Date(a.createdAt).getTime() : 0;
      const tb = b.createdAt ? new Date(b.createdAt).getTime() : 0;
      return ta - tb;
    });
  }, [data?.runs]);

  const isLoading = enabled && (isPending || (isFetching && !data));
  const totalCount = runs.length > 0 ? runs.length : (data?.jobs?.length ?? 0);
  const hasAny = totalCount > 0 || runs.length > 0;

  if (!enabled || taskId === 0) {
    return null;
  }

  if (isLoading) {
    return (
      <Collapsible open={sectionOpen} onOpenChange={setSectionOpen} className="flex flex-col gap-2">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-muted-foreground">Agent Runs</span>
            <Skeleton className="h-5 w-8 rounded-full" />
          </div>
          <CollapsibleTrigger asChild>
            <Button variant="ghost" size="icon-sm" className="size-6">
              <ChevronDown className={cn("transition-transform duration-200", sectionOpen && "rotate-180")} />
            </Button>
          </CollapsibleTrigger>
        </div>
        <CollapsibleContent className="flex flex-col gap-2 data-[state=open]:animate-collapsible-down data-[state=closed]:animate-collapsible-up">
          <Skeleton className="h-20 w-full" />
          <Skeleton className="h-20 w-full" />
        </CollapsibleContent>
      </Collapsible>
    );
  }

  const showSection = hasAny || (data !== undefined && (data.jobs.length > 0 || runs.length > 0));
  if (!showSection && data && data.jobs.length === 0 && runs.length === 0) {
    return (
      <Collapsible open={sectionOpen} onOpenChange={setSectionOpen} className="flex flex-col gap-2">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-muted-foreground">Agent Runs</span>
            <Badge variant="secondary" className="text-xs">0</Badge>
          </div>
          <CollapsibleTrigger asChild>
            <Button variant="ghost" size="icon-sm" className="size-6">
              <ChevronDown className={cn("transition-transform duration-200", sectionOpen && "rotate-180")} />
            </Button>
          </CollapsibleTrigger>
        </div>
        <CollapsibleContent className="data-[state=open]:animate-collapsible-down data-[state=closed]:animate-collapsible-up">
          <p className="text-sm text-muted-foreground pt-1">No agent runs yet</p>
        </CollapsibleContent>
      </Collapsible>
    );
  }

  return (
    <Collapsible open={sectionOpen} onOpenChange={setSectionOpen} className="flex flex-col gap-2">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-muted-foreground">Agent Runs</span>
          <Badge variant="secondary" className="text-xs">{totalCount}</Badge>
        </div>
        <CollapsibleTrigger asChild>
          <Button variant="ghost" size="icon-sm" className="size-6">
            <ChevronDown className={cn("transition-transform duration-200", sectionOpen && "rotate-180")} />
          </Button>
        </CollapsibleTrigger>
      </div>
      <CollapsibleContent className="flex flex-col gap-3 pt-1 data-[state=open]:animate-collapsible-down data-[state=closed]:animate-collapsible-up">
        {runs.length === 0 ? (
          <>
            <p className="text-sm text-muted-foreground">
              No agent runs yet
              {data && data.jobs.length > 0 && (
                <span className="block text-xs mt-1">
                  {data.jobs.length} job{data.jobs.length !== 1 ? "s" : ""} queued — run will appear here when the worker picks it up.
                </span>
              )}
            </p>
            {data && data.jobs.length > 0 && (
              <p className="text-xs text-muted-foreground">
                Branch <span className="font-mono">aycorn/task-{taskId}</span> will be left for manual merge — no auto-merge.
              </p>
            )}
          </>
        ) : (
          runs.map((run: AgentRun) => <RunCard key={run.id} run={run} taskId={taskId} />)
        )}
      </CollapsibleContent>
    </Collapsible>
  );
}
