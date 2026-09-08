import { useState } from "react";
import {
  ArrowDown,
  Check,
  FileDiff,
  GitBranch,
  GitMerge,
  Loader2,
  RefreshCw,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogCancel,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  useMergeBranch,
  useMergePreview,
} from "@/features/task/branches/queries/use-task-branches";
import type { TaskBranch } from "@/features/task/branches/types";

export function MergeBranchDialog({
  branch,
  onClose,
  onReturnFocus,
}: {
  branch: TaskBranch;
  onClose: () => void;
  onReturnFocus: () => void;
}) {
  const [target, setTarget] = useState("");
  const preview = useMergePreview(branch.taskId, branch.jobId, target);
  const merge = useMergeBranch(branch.taskId, branch.jobId);
  const data = preview.data;
  const busy = merge.isPending;
  const mergeDisabled =
    busy ||
    preview.isFetching ||
    !!preview.error ||
    !data?.token ||
    !!data.blocked ||
    data.alreadyMerged;
  const error = merge.error?.message || preview.error?.message;

  return (
    <AlertDialog
      open
      onOpenChange={(open) => {
        if (!open && !busy) onClose();
      }}
    >
      <AlertDialogContent
        className="max-h-[85dvh] overflow-y-auto p-4 outline-none sm:p-6 sm:data-[size=default]:max-w-xl"
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          onReturnFocus();
        }}
        onEscapeKeyDown={(event) => {
          if (busy) event.preventDefault();
        }}
      >
        <AlertDialogHeader>
          <AlertDialogTitle className="flex items-center gap-2">
            <GitMerge className="size-5 text-primary" /> Merge branch
          </AlertDialogTitle>
          <AlertDialogDescription>
            Review this task’s code and choose where it belongs.
          </AlertDialogDescription>
        </AlertDialogHeader>

        <div className="rounded-xl border border-border bg-muted/30 p-4">
          <p className="mb-2 text-xs font-medium text-muted-foreground">
            FROM · AGENT RUN #{branch.jobId}
          </p>
          <div className="flex items-start gap-2 text-sm">
            <GitBranch className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
            <code className="break-all">{branch.branch}</code>
          </div>
          <ArrowDown className="my-3 size-4 text-muted-foreground" />
          <label
            htmlFor={`merge-target-${branch.jobId}`}
            className="mb-2 block text-xs font-medium text-muted-foreground"
          >
            INTO
          </label>
          <Select
            value={target || data?.target || ""}
            onValueChange={(value) => {
              setTarget(value);
              merge.reset();
            }}
            disabled={busy || !data?.targets.length}
          >
            <SelectTrigger
              id={`merge-target-${branch.jobId}`}
              className="w-full bg-background"
            >
              <SelectValue placeholder="Choose a local branch" />
            </SelectTrigger>
            <SelectContent>
              {data?.targets.map((name) => (
                <SelectItem key={name} value={name}>
                  <span className="break-all font-mono">{name}</span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        {preview.isPending ? (
          <p
            role="status"
            className="flex items-center gap-2 text-sm text-muted-foreground"
          >
            <Loader2 className="size-4 animate-spin motion-reduce:animate-none" />{" "}
            Reading Git changes…
          </p>
        ) : (
          data && (
            <div className="space-y-3 text-sm">
              <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
                <span className="flex items-center gap-1.5">
                  <FileDiff className="size-4 text-muted-foreground" />
                  {data.files.length}{" "}
                  {data.files.length === 1 ? "file" : "files"} changed
                </span>
                <span className="text-muted-foreground">
                  {data.commits} {data.commits === 1 ? "commit" : "commits"}
                  {data.uncommitted ? " + uncommitted edits" : ""}
                </span>
              </div>
              {data.uncommitted && (
                <p className="rounded-lg bg-muted p-3 text-muted-foreground">
                  This will commit all uncommitted edits and new, non-ignored
                  files in the agent workspace before merging.
                </p>
              )}
              {data.alreadyMerged && (
                <p className="flex items-center gap-2 text-muted-foreground">
                  <Check className="size-4" /> This branch is already included
                  in {data.target}.
                </p>
              )}
              {data.blocked && (
                <p
                  role="alert"
                  className="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-destructive"
                >
                  {data.blocked}
                </p>
              )}
              {!!data.files.length && (
                <details className="rounded-lg border border-border">
                  <summary className="cursor-pointer px-3 py-2 font-medium">
                    Review changed files
                  </summary>
                  <ul className="max-h-32 overflow-auto border-t border-border px-3 py-2 font-mono text-xs text-muted-foreground">
                    {data.files.map((file) => (
                      <li key={file} className="break-all py-0.5">
                        {file}
                      </li>
                    ))}
                  </ul>
                  {data.diff && (
                    <pre
                      tabIndex={0}
                      aria-label="Git diff"
                      className="max-h-64 overflow-auto border-t border-border bg-muted/40 p-3 text-xs"
                    >
                      <code>{data.diff}</code>
                    </pre>
                  )}
                </details>
              )}
            </div>
          )
        )}
        {error && (
          <p
            role="alert"
            className="whitespace-pre-wrap break-words text-sm text-destructive"
          >
            {error}
          </p>
        )}
        <p className="text-xs text-muted-foreground">
          Merges locally using Git. Both branches are kept. If Git finds
          conflicts, the destination stays unchanged.
        </p>
        <AlertDialogFooter className="items-stretch sm:items-center">
          <Button
            variant="ghost"
            size="sm"
            className="sm:mr-auto"
            disabled={busy || preview.isFetching}
            onClick={() => {
              merge.reset();
              void preview.refetch();
            }}
          >
            <RefreshCw
              className={
                preview.isFetching
                  ? "animate-spin motion-reduce:animate-none"
                  : ""
              }
            />{" "}
            Refresh
          </Button>
          <AlertDialogCancel disabled={busy}>Cancel</AlertDialogCancel>
          <Button
            disabled={mergeDisabled}
            onClick={() => {
              if (data) merge.mutate(data, { onSuccess: onClose });
            }}
          >
            {busy ? (
              <Loader2 className="animate-spin motion-reduce:animate-none" />
            ) : (
              <GitMerge />
            )}
            {busy
              ? "Merging…"
              : data?.uncommitted
                ? "Commit & merge"
                : "Merge branch"}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
