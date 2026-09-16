import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  ExternalLink,
  FileText,
  Loader2,
  Pin,
  Play,
  RotateCw,
  Square,
  Trash2,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  environmentRequest,
  useEnvironmentMutations,
  type Environment,
} from "./use-environments";

const environmentBusy = (state: string) =>
  [
    "queued",
    "snapshotting",
    "building",
    "starting",
    "stopping",
    "deleting",
  ].includes(state);

function EnvironmentLogs({
  environment,
  onClose,
}: {
  environment: Environment;
  onClose: () => void;
}) {
  const query = useQuery({
    queryKey: ["environment-logs", environment.id],
    queryFn: ({ signal }) =>
      environmentRequest<{ logs: string }>(
        `/api/environment/${environment.id}/logs`,
        "GET",
        undefined,
        signal,
      ),
    refetchInterval: 5000,
  });
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="max-w-4xl sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>Logs · {environment.name}</DialogTitle>
          <DialogDescription>
            Build output, test results, application logs, and cluster warnings.
          </DialogDescription>
        </DialogHeader>
        {query.error && (
          <p role="alert" className="text-sm text-destructive">
            {query.error.message}
          </p>
        )}
        <pre
          aria-live="off"
          tabIndex={0}
          className="max-h-[65vh] overflow-auto whitespace-pre-wrap break-words rounded-lg bg-muted p-4 font-mono text-xs"
        >
          {query.data?.logs ||
            (query.isPending ? "Loading logs…" : "Waiting for output…")}
        </pre>
        <Button
          variant="outline"
          onClick={() => void query.refetch()}
          disabled={query.isFetching}
        >
          Refresh logs
        </Button>
      </DialogContent>
    </Dialog>
  );
}

export function EnvironmentCard({
  environment: e,
}: {
  environment: Environment;
}) {
  const { action, edit } = useEnvironmentMutations(e.projectId);
  const [logsOpen, setLogsOpen] = useState(false);
  const [confirm, setConfirm] = useState<"delete" | "rebuild" | null>(null);
  const source = useQuery({
    queryKey: ["environment-source", e.id],
    queryFn: () =>
      environmentRequest<{ changed: boolean; message: string }>(
        `/api/environment/${e.id}/source`,
      ),
    enabled: false,
    retry: false,
  });
  const busy = action.isPending || e.desired === "deleted";
  return (
    <div className="min-w-0 space-y-3 p-4">
      <div className="flex flex-wrap items-start gap-2">
        <Input
          key={e.name}
          aria-label={`Environment ${e.id} name`}
          defaultValue={e.name}
          maxLength={120}
          className="h-8 min-w-0 flex-1 border-transparent bg-transparent px-1 font-medium shadow-none hover:border-input focus:border-input"
          onBlur={(event) => {
            const name = event.target.value.trim();
            if (name && name !== e.name) edit.mutate({ id: e.id, name });
            else event.target.value = e.name;
          }}
        />
        <Badge
          variant="secondary"
          role="status"
          className={`gap-1.5 capitalize ${e.state === "failed" ? "text-destructive" : ""}`}
        >
          {environmentBusy(e.state) && (
            <Loader2 className="size-3 animate-spin motion-reduce:animate-none" />
          )}
          {e.state}
        </Badge>
      </div>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
        <code className="break-all">
          {e.branch} · {e.commit.slice(0, 8)}
        </code>
        {e.includeChanges && <span>Includes captured local edits</span>}
        <span>
          Tests: {e.testState}
          {e.testExitCode !== null ? ` · exit ${e.testExitCode}` : ""}
        </span>
      </div>
      {e.error && (
        <p
          role="alert"
          className="line-clamp-4 break-words text-sm text-destructive"
        >
          {e.error.length > 2048 ? `… ${e.error.slice(-2048)}` : e.error}
        </p>
      )}
      <div className="flex flex-wrap items-center gap-2">
        {e.state === "ready" && e.url && (
          <Button asChild size="sm">
            <a
              href={e.url}
              target="_blank"
              rel="noopener noreferrer"
              onClick={() => action.mutate({ id: e.id, action: "touch" })}
            >
              <ExternalLink className="size-4" />
              Open preview
            </a>
          </Button>
        )}
        <Button variant="outline" size="sm" onClick={() => setLogsOpen(true)}>
          <FileText className="size-4" />
          Logs
        </Button>
        {e.state === "stopped" && e.testState !== "failed" && (
          <Button
            variant="outline"
            size="sm"
            disabled={busy}
            onClick={() => action.mutate({ id: e.id, action: "start" })}
          >
            <Play className="size-4" />
            Start
          </Button>
        )}
        {e.desired === "running" && (
          <Button
            variant="outline"
            size="sm"
            disabled={busy}
            onClick={() => action.mutate({ id: e.id, action: "stop" })}
          >
            <Square className="size-4" />
            Stop
          </Button>
        )}
        <Button
          variant="ghost"
          size="sm"
          disabled={busy}
          onClick={() => setConfirm("rebuild")}
        >
          <RotateCw className="size-4" />
          Rebuild
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="size-8"
          aria-label={
            e.pinned ? "Unpin environment" : "Keep environment running"
          }
          aria-pressed={e.pinned}
          disabled={busy || edit.isPending}
          onClick={() => edit.mutate({ id: e.id, pinned: !e.pinned })}
        >
          <Pin
            className={`size-4 ${e.pinned ? "fill-current text-primary" : ""}`}
          />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="size-8 text-destructive"
          aria-label={`Delete environment ${e.name}`}
          disabled={busy}
          onClick={() => setConfirm("delete")}
        >
          <Trash2 className="size-4" />
        </Button>
      </div>
      {e.testState === "failed" && (
        <p className="text-xs text-muted-foreground">
          Tests failed. Rebuild to test a fresh snapshot.
        </p>
      )}
      <p className="text-xs text-muted-foreground">
        {e.pinned
          ? "Pinned · stays running until you stop it"
          : e.desired === "running"
            ? `Auto-stops ${new Date(e.expiresAt * 1000).toLocaleString()}`
            : "Stopped data is retained until you delete the environment"}
      </p>
      <details className="text-xs text-muted-foreground">
        <summary className="w-fit cursor-pointer">Source and resources</summary>
        <dl className="mt-2 space-y-1">
          <dt>Snapshot SHA-256</dt>
          <dd className="break-all font-mono">{e.digest || "Capturing…"}</dd>
          <dt>Cluster</dt>
          <dd>{e.settings.context}</dd>
          <dt>Limits</dt>
          <dd>
            {e.settings.cpuMillis / 1000} CPU · {e.settings.memoryMiB} MiB
            memory · {e.settings.storageGiB} GiB data
          </dd>
          <dt>Created</dt>
          <dd>{new Date(e.createdAt * 1000).toLocaleString()}</dd>
        </dl>
        <Button
          size="sm"
          variant="link"
          className="px-0"
          disabled={!e.digest || source.isFetching}
          onClick={() => void source.refetch()}
        >
          Check for newer code
        </Button>
        {(source.data || source.error) && (
          <p role="status">{source.data?.message || source.error?.message}</p>
        )}
      </details>
      <AlertDialog
        open={confirm !== null}
        onOpenChange={(open) => {
          if (!open) setConfirm(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {confirm === "delete"
                ? "Delete this environment and its data?"
                : "Build a fresh preview?"}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirm === "delete"
                ? "This permanently deletes this preview’s database and Kubernetes resources. The source branch and task worktree are kept."
                : "Captures the latest code using your current project recipe and creates a separate environment with fresh data. The existing preview and its data are retained."}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant={confirm === "delete" ? "destructive" : "default"}
              onClick={() => {
                if (confirm) action.mutate({ id: e.id, action: confirm });
                setConfirm(null);
              }}
            >
              {confirm === "delete"
                ? "Delete environment"
                : "Build fresh preview"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      {logsOpen && (
        <EnvironmentLogs environment={e} onClose={() => setLogsOpen(false)} />
      )}
    </div>
  );
}
