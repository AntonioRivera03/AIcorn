import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ArrowRightLeft,
  CalendarClock,
  FileText,
  LoaderCircle,
  Pencil,
  Play,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { jobRequest, useRunJob, type ScheduledJob } from "./use-jobs";
import type { JobGridItem } from "./job-grid-items";

export function JobGridCard({
  item,
  projectId,
  edit,
}: {
  item: JobGridItem;
  projectId: number;
  edit: (selection: { kind: "templates" | "jobs"; id: number }) => void;
}) {
  const client = useQueryClient();
  const job = item.kind === "jobs" ? item.job : undefined;
  const template = item.template;
  const name = job?.name || template?.name || "Untitled";
  const run = useRunJob(projectId, job?.id ?? 0);
  const convert = useMutation({
    mutationFn: async () => {
      if (job) {
        await jobRequest<void>(
          `/api/project/${projectId}/automation/jobs/${job.id}`,
          "DELETE",
        );
        return null;
      }
      return jobRequest<ScheduledJob>(
        `/api/project/${projectId}/automation/templates/${template!.id}/job`,
        "POST",
      );
    },
    onSuccess: (created) => {
      client.setQueryData<ScheduledJob[]>(
        ["scheduled-jobs", projectId],
        (items) =>
          created
            ? [created, ...(items ?? [])]
            : items?.filter((item) => item.id !== job!.id),
      );
      if (created) edit({ kind: "jobs", id: created.id });
      toast.success(created ? "Converted to a job" : "Returned to a template");
    },
    onError: (error: Error) => toast.error(error.message),
  });
  const summary =
    template?.body?.replace(/[#*`>_[\]]/g, "").trim() ||
    template?.title ||
    "Add task details to make this reusable.";
  const status = !job
    ? "Reusable task"
    : !job.schedule
      ? "Run manually"
      : job.enabled
        ? "Scheduled"
        : "Schedule paused";
  return (
    <article
      aria-label={`${job ? "Job" : "Template"}: ${name}`}
      className="flex min-w-0 flex-col gap-4 rounded-lg border border-border bg-card p-4 shadow-sm sm:flex-row sm:items-center"
    >
      <div className="flex min-w-0 flex-1 items-start gap-3">
        <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          {job ? (
            <CalendarClock className="size-5" />
          ) : (
            <FileText className="size-5" />
          )}
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h3 className="truncate text-sm font-semibold" title={name}>
              {name}
            </h3>
            <Badge variant="outline" className="shrink-0 font-normal">
              {job ? "Job" : "Template"}
            </Badge>
          </div>
          <p className="mt-1 line-clamp-2 text-sm leading-5 text-muted-foreground sm:line-clamp-1">
            {summary}
          </p>
          <div className="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
            <span>{status}</span>
            {job?.schedule && (
              <>
                <span aria-hidden="true">·</span>
                <span className="font-mono">{job.schedule}</span>
                <span>{job.timezone}</span>
              </>
            )}
            {!job && template?.priority && (
              <>
                <span aria-hidden="true">·</span>
                <span>{template.priority} priority</span>
              </>
            )}
          </div>
          {job?.lastError && (
            <p className="mt-2 line-clamp-2 text-xs text-destructive">
              {job.lastError}
            </p>
          )}
        </div>
      </div>
      <div className="flex shrink-0 items-center justify-end gap-2">
        {job && (
          <Button
            size="icon-sm"
            disabled={run.isPending || convert.isPending || !job.agentId}
            onClick={() => run.mutate()}
            aria-label={`Run ${name}`}
            aria-busy={run.isPending}
            title={
              !job.agentId
                ? "Choose an agent in Edit before running"
                : run.isPending
                  ? "Queuing…"
                  : "Run"
            }
          >
            {run.isPending ? (
              <LoaderCircle className="size-3.5 animate-spin motion-reduce:animate-none" />
            ) : (
              <Play className="size-3.5" />
            )}
          </Button>
        )}
        {job ? (
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                disabled={convert.isPending || run.isPending || !template}
                aria-label={`Convert ${name} to template`}
                title="Convert to template"
              >
                <ArrowRightLeft className="size-3.5" />
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>
                  Convert “{name}” to a template?
                </AlertDialogTitle>
                <AlertDialogDescription>
                  This removes the job’s schedule and run history. Its task
                  template and previously created tasks remain. A job with an
                  active run cannot be converted.
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>Cancel</AlertDialogCancel>
                <AlertDialogAction onClick={() => convert.mutate()}>
                  Convert to template
                </AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        ) : (
          <Button
            variant="ghost"
            size="icon-sm"
            disabled={convert.isPending}
            onClick={() => convert.mutate()}
            aria-label={`Convert ${name} to job`}
            title="Convert to job"
          >
            <ArrowRightLeft className="size-3.5" />
          </Button>
        )}
        <Button
          variant="outline"
          size="icon-sm"
          onClick={() => edit({ kind: item.kind, id: job?.id ?? template!.id })}
          aria-label={`Edit ${name}`}
          title="Edit"
        >
          <Pencil className="size-3.5" />
        </Button>
      </div>
    </article>
  );
}
