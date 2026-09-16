import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ArrowRightLeft,
  CalendarClock,
  FileText,
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
      className="flex min-w-0 flex-col rounded-xl border border-border bg-card p-5 shadow-sm transition-shadow hover:shadow-md"
    >
      <div className="mb-4 flex items-center justify-between gap-3">
        <div className="flex size-10 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          {job ? (
            <CalendarClock className="size-5" />
          ) : (
            <FileText className="size-5" />
          )}
        </div>
        <Badge variant="outline" className="font-normal">
          {job ? "Job" : "Template"}
        </Badge>
      </div>
      <h3 className="truncate text-base font-semibold" title={name}>
        {name}
      </h3>
      <p className="mt-2 line-clamp-2 min-h-10 text-sm leading-5 text-muted-foreground">
        {summary}
      </p>
      <div className="mb-5 mt-4 flex min-h-5 flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
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
        <p className="mb-3 line-clamp-2 text-xs text-destructive">
          {job.lastError}
        </p>
      )}
      <div className="mt-auto flex flex-wrap items-center gap-2 border-t border-border pt-4">
        <Button
          variant="outline"
          size="sm"
          onClick={() => edit({ kind: item.kind, id: job?.id ?? template!.id })}
          aria-label={`Edit ${name}`}
        >
          <Pencil className="size-3.5" />
          Edit
        </Button>
        {job ? (
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                disabled={convert.isPending || run.isPending || !template}
                aria-label={`Convert ${name} to template`}
              >
                <ArrowRightLeft className="size-3.5" />
                Convert
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
            size="sm"
            disabled={convert.isPending}
            onClick={() => convert.mutate()}
            aria-label={`Convert ${name} to job`}
          >
            <ArrowRightLeft className="size-3.5" />
            Convert
          </Button>
        )}
        {job && (
          <Button
            size="sm"
            className="ml-auto"
            disabled={run.isPending || convert.isPending || !job.agentId}
            onClick={() => run.mutate()}
            aria-label={`Run ${name}`}
            title={
              !job.agentId
                ? "Choose an agent in Edit before running"
                : undefined
            }
          >
            <Play className="size-3.5" />
            {run.isPending ? "Queuing…" : "Run"}
          </Button>
        )}
      </div>
    </article>
  );
}
