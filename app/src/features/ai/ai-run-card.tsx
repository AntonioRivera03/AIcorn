import { useEffect, useState } from "react";
import { Copy, RotateCcw, Square, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { AIMarkdown } from "@/features/ai/ai-markdown";
import {
  isAgentWorking,
  type AgentJob,
  type AgentRun,
} from "@/features/agentJob/queries/useAgentJobs";

const statusLabels: Record<AgentJob["status"], string> = {
  pending: "Queued",
  claimed: "Starting",
  running: "Running",
  completed: "Result ready",
  failed: "Failed",
  canceled: "Stopped",
  canceling: "Stopping",
  interrupted: "Interrupted",
};
const copy = async (text: string) => {
  try {
    await navigator.clipboard.writeText(text);
    toast.success("Copied");
  } catch {
    toast.error("Could not copy");
  }
};

export function AIRunCard({
  job,
  run,
  onStop,
  onRetry,
  busy,
}: {
  job: AgentJob;
  run?: AgentRun;
  onStop: () => void;
  onRetry: () => void;
  busy: boolean;
}) {
  const active = isAgentWorking([job]);
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!active) return;
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [active]);
  const seconds = job.startedAt
    ? Math.max(
        0,
        Math.floor(
          ((job.finishedAt ? new Date(job.finishedAt).getTime() : now) -
            new Date(job.startedAt).getTime()) /
            1000,
        ),
      )
    : null;
  const elapsed =
    seconds === null ? "" : `${Math.floor(seconds / 60)}m ${seconds % 60}s`;

  const artifacts = run?.artifacts;
  const legacy = !job.request;
  return (
    <article
      className="rounded-xl border border-border bg-card p-4"
      aria-label={`AI run ${job.id}`}
    >
      <div className="flex flex-wrap items-center gap-2">
        <Badge
          variant={job.status === "failed" ? "destructive" : "secondary"}
          className="gap-1.5"
        >
          {active && job.status !== "pending" && (
            <Loader2 className="size-3 animate-spin motion-reduce:animate-none" />
          )}
          {statusLabels[job.status] || job.status}
        </Badge>
        <span className="text-sm capitalize">
          {job.request?.intent || "Previous run"}
        </span>
        {elapsed && (
          <span
            className="text-xs text-muted-foreground"
            aria-label="Run duration"
          >
            {elapsed}
          </span>
        )}
        <time className="ml-auto text-xs text-muted-foreground">
          {job.createdAt ? new Date(job.createdAt).toLocaleString() : ""}
        </time>
      </div>
      {job.request?.instruction && (
        <p className="mt-3 whitespace-pre-wrap text-sm text-muted-foreground">
          {job.request.instruction}
        </p>
      )}
      {active && (
        <p className="mt-3 text-sm text-muted-foreground" role="status">
          {job.progress ||
            (job.status === "pending"
              ? "Waiting for the current run to finish…"
              : "Starting…")}
        </p>
      )}
      {job.error && (
        <p
          role="alert"
          className="mt-3 whitespace-pre-wrap text-sm text-destructive"
        >
          {job.error}
        </p>
      )}
      {run?.output && <AIMarkdown>{run.output}</AIMarkdown>}
      {!active && !run?.output && (
        <p className="mt-3 text-sm text-muted-foreground">
          No response was recorded for this attempt.
        </p>
      )}
      {artifacts?.warning && (
        <p className="mt-3 text-sm text-destructive">{artifacts.warning}</p>
      )}
      {artifacts?.workspace && (
        <details className="mt-4 rounded-lg border border-border p-3">
          <summary className="cursor-pointer font-medium">
            Changes{" "}
            {artifacts.files?.length ? `· ${artifacts.files.length} files` : ""}
          </summary>
          {artifacts.files?.length ? (
            <ul className="mt-3 space-y-1 text-xs font-mono">
              {artifacts.files.map((file) => (
                <li key={file} className="break-all">
                  {file}
                </li>
              ))}
            </ul>
          ) : (
            <p className="mt-3 text-sm text-muted-foreground">
              No file changes recorded.
            </p>
          )}
          {artifacts.diff && (
            <pre className="mt-3 max-h-96 overflow-auto rounded bg-muted p-3 text-xs">
              <code>{artifacts.diff}</code>
            </pre>
          )}
        </details>
      )}
      <div className="mt-4 flex flex-wrap items-center gap-2">
        {active ? (
          <Button
            variant="outline"
            size="sm"
            onClick={onStop}
            disabled={busy || job.status === "canceling"}
          >
            <Square className="size-3" />{" "}
            {job.status === "canceling" ? "Stopping…" : "Stop"}
          </Button>
        ) : (
          <Button variant="ghost" size="sm" onClick={onRetry} disabled={busy}>
            <RotateCcw className="size-3" /> Use request again
          </Button>
        )}
        {run?.output && (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => void copy(run.output)}
          >
            <Copy className="size-3" /> Copy answer
          </Button>
        )}
      </div>
      <details className="mt-3 text-xs text-muted-foreground">
        <summary className="cursor-pointer">Details</summary>
        <dl className="mt-3 grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 break-all">
          <dt>Run</dt>
          <dd>
            {job.id}
            {legacy ? " · legacy metadata unavailable" : ""}
          </dd>
          {job.request && (
            <>
              <dt>Engine</dt>
              <dd>OpenCode {job.request.engineVersion}</dd>
              <dt>Model</dt>
              <dd>{job.request.model}</dd>
              <dt>Preset</dt>
              <dd>{job.request.presetName || "None"}</dd>
            </>
          )}
          {artifacts?.workspace && (
            <>
              <dt>Workspace</dt>
              <dd className="select-all">{artifacts.workspace}</dd>
              <dt>Branch</dt>
              <dd className="select-all">{artifacts.branch}</dd>
              <dt>Base commit</dt>
              <dd>{artifacts.baseCommit}</dd>
            </>
          )}
        </dl>
        {run?.usageJson && run.usageJson !== "{}" && (
          <pre className="mt-3 max-h-40 overflow-auto whitespace-pre-wrap rounded bg-muted p-2">
            {run.usageJson}
          </pre>
        )}
      </details>
    </article>
  );
}
