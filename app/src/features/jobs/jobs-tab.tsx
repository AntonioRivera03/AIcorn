import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import { Link } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  ArrowLeft,
  CalendarClock,
  FilePlus2,
  Play,
  Plus,
  Search,
  Trash2,
  X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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
import { useProjectWorkflowSettingsQuery } from "@/features/settings/project-workflow/queries/useProjectWorkflowSettingsQuery";
import { usePersonasQuery } from "@/features/persona/queries/use-personas-query";
import type { Checklist, TaskType } from "@/types/types";
import {
  jobRequest,
  useJobEdit,
  useJobResources,
  useRunJob,
  type JobRun,
  type ScheduledJob,
  type TaskTemplate,
} from "./use-jobs";
import { JobGridCard } from "./job-grid-card";
import { jobGridItems } from "./job-grid-items";

// Track every field independently: saving a name must not dismiss a failed cron edit.
const PendingFields = createContext<
  ((id: string, pending: boolean) => void) | null
>(null);
function usePendingFields() {
  const [fields, setFields] = useState<Record<string, boolean>>({});
  const update = useCallback((id: string, pending: boolean) => {
    setFields((current) =>
      current[id] === pending ? current : { ...current, [id]: pending },
    );
  }, []);
  return { update, pending: Object.values(fields).some(Boolean) };
}

function AutoField({
  label,
  value,
  save,
  multiline = false,
  placeholder,
}: {
  label: string;
  value: string;
  save: (value: string) => Promise<unknown>;
  multiline?: boolean;
  placeholder?: string;
}) {
  const id = useId();
  const [draft, setDraft] = useState<string | null>(null);
  const [error, setError] = useState("");
  const pending = useRef<string | null>(null);
  const updatePending = useContext(PendingFields);
  useEffect(() => {
    updatePending?.(id, draft !== null && (draft !== value || !!error));
  }, [draft, error, id, updatePending, value]);
  useEffect(() => () => updatePending?.(id, false), [id, updatePending]);
  async function commit() {
    if (
      draft === null ||
      (draft === value && !error) ||
      draft === pending.current
    )
      return;
    const sent = draft;
    pending.current = sent;
    try {
      await save(sent);
      setDraft((current) => (current === sent ? null : current));
      setError("");
    } catch (e) {
      setError(
        e instanceof Error
          ? e.message
          : "Could not save. Edit or leave this field to retry.",
      );
    } finally {
      if (pending.current === sent) pending.current = null;
    }
  }
  const props = {
    id,
    value: draft ?? value,
    placeholder,
    onChange: (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
      setDraft(e.target.value),
    onBlur: () => void commit(),
    onKeyDown: (e: React.KeyboardEvent) => {
      if (e.key === "Escape") {
        setDraft(null);
        setError("");
      }
    },
  };
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label}</Label>
      {multiline ? (
        <Textarea {...props} className="min-h-28 resize-y" />
      ) : (
        <Input {...props} />
      )}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
    </div>
  );
}
function Choice({
  label,
  value,
  options,
  change,
  disabled,
}: {
  label: string;
  value: string | number;
  options: { value: string | number; label: string }[];
  change: (value: string) => void;
  disabled?: boolean;
}) {
  const id = useId();
  return (
    <div className="space-y-2">
      <Label htmlFor={id}>{label}</Label>
      <select
        id={id}
        value={value}
        disabled={disabled}
        onChange={(e) => change(e.target.value)}
        className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
      >
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    </div>
  );
}
function DeleteEntity({
  name,
  pending,
  remove,
}: {
  name: string;
  pending: boolean;
  remove: () => void;
}) {
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button variant="outline" size="sm" disabled={pending}>
          <Trash2 className="size-4" />
          Delete
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete {name || "this item"}?</AlertDialogTitle>
          <AlertDialogDescription>
            This cannot be undone. Tasks already created from it remain on the
            board. Jobs using a template must be deleted first; active jobs
            cannot be deleted.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction onClick={remove}>Delete</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

type Selection = { kind: "templates" | "jobs"; id: number };
export function JobsTab({ projectId }: { projectId: number }) {
  const client = useQueryClient();
  const { templates, jobs } = useJobResources(projectId);
  const [selection, setSelection] = useState<Selection | null>(null);
  const [search, setSearch] = useState("");
  const editorPanel = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (selection) editorPanel.current?.focus();
  }, [selection]);
  const create = useMutation({
    mutationFn: () =>
      jobRequest<TaskTemplate>(
        `/api/project/${projectId}/automation/templates`,
        "POST",
      ),
    onSuccess: (template) => {
      client.setQueryData<TaskTemplate[]>(
        ["task-templates", projectId],
        (old) => [template, ...(old ?? [])],
      );
      setSelection({ kind: "templates", id: template.id });
    },
    onError: (error: Error) => toast.error(error.message),
  });
  if (templates.isPending || jobs.isPending)
    return (
      <p className="p-6 text-sm text-muted-foreground">
        Loading templates and jobs…
      </p>
    );
  if (!templates.data || !jobs.data)
    return (
      <div role="alert" className="p-6 text-destructive">
        Could not load jobs.{" "}
        <Button
          variant="outline"
          onClick={() => {
            void templates.refetch();
            void jobs.refetch();
          }}
        >
          Retry
        </Button>
      </div>
    );
  const template =
    selection?.kind === "templates"
      ? templates.data.find((t) => t.id === selection.id)
      : undefined;
  const job =
    selection?.kind === "jobs"
      ? jobs.data.find((j) => j.id === selection.id)
      : undefined;
  if (template || job)
    return (
      <div ref={editorPanel} tabIndex={-1} className="pb-8 outline-none">
        {template ? (
          <TemplateEditor
            key={`template-${template.id}`}
            template={template}
            select={setSelection}
            removed={() => setSelection(null)}
          />
        ) : (
          job && (
            <JobEditor
              key={`job-${job.id}`}
              job={job}
              template={templates.data.find((t) => t.id === job.templateId)}
              select={setSelection}
              removed={() => setSelection(null)}
            />
          )
        )}
      </div>
    );
  const items = jobGridItems(templates.data, jobs.data, search);
  const total = jobGridItems(templates.data, jobs.data, "").length;
  return (
    <section className="space-y-6 pb-8">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h2 className="text-xl font-semibold tracking-tight">
            Jobs & templates
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            Reusable tasks and automated work, all in one place.
          </p>
        </div>
        <Button onClick={() => create.mutate()} disabled={create.isPending}>
          <Plus className="size-4" />
          New template
        </Button>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="relative w-full sm:max-w-sm">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            aria-label="Search jobs and templates"
            placeholder="Search jobs and templates…"
            className="pl-9 pr-9"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
          {search && (
            <Button
              variant="ghost"
              size="icon-sm"
              className="absolute right-1 top-1/2 -translate-y-1/2"
              aria-label="Clear search"
              onClick={() => setSearch("")}
            >
              <X className="size-3.5" />
            </Button>
          )}
        </div>
        <p role="status" className="text-xs text-muted-foreground">
          {search ? `${items.length} of ${total}` : total}{" "}
          {total === 1 ? "item" : "items"}
        </p>
      </div>
      {items.length ? (
        <div
          className="grid gap-4 md:grid-cols-2 xl:grid-cols-3"
          aria-label="Jobs and templates"
        >
          {items.map((item) => (
            <JobGridCard
              key={`${item.kind}-${item.kind === "jobs" ? item.job.id : item.template.id}`}
              item={item}
              projectId={projectId}
              edit={setSelection}
            />
          ))}
        </div>
      ) : (
        <div className="rounded-xl border border-dashed border-border px-6 py-16 text-center">
          <FilePlus2 className="mx-auto mb-3 size-7 text-muted-foreground" />
          <h3 className="font-medium">
            {search ? "No matches" : "Start with a template"}
          </h3>
          <p className="mt-1 text-sm text-muted-foreground">
            {search
              ? "Try a different name or task detail."
              : "Create a reusable task, then convert it to a job whenever you’re ready."}
          </p>
        </div>
      )}
    </section>
  );
}

function TemplateEditor({
  template: t,
  select,
  removed,
}: {
  template: TaskTemplate;
  select: (selection: Selection) => void;
  removed: () => void;
}) {
  const client = useQueryClient();
  const fields = usePendingFields();
  const edit = useJobEdit<TaskTemplate>(t.projectId, t.id, "templates");
  const workflow = useProjectWorkflowSettingsQuery(t.projectId);
  const checklists = useQuery({
    queryKey: ["job-checklists", t.projectId],
    queryFn: () =>
      jobRequest<Checklist[]>(`/api/project/checklist/${t.projectId}`),
  });
  const types = useQuery({
    queryKey: ["projectTaskTypes", t.projectId],
    queryFn: () =>
      jobRequest<TaskType[]>(
        `/api/project/${t.projectId}/settings/task-types/enabled`,
      ),
  });
  const action = useMutation({
    scope: { id: `templates-${t.projectId}-${t.id}` },
    mutationFn: async (kind: "instantiate" | "job" | "delete") => {
      const url = `/api/project/${t.projectId}/automation/templates/${t.id}`;
      if (kind === "delete") {
        await jobRequest<void>(url, "DELETE");
        return;
      }
      if (kind === "job") {
        const job = await jobRequest<ScheduledJob>(`${url}/job`, "POST");
        client.setQueryData<ScheduledJob[]>(
          ["scheduled-jobs", t.projectId],
          (old) => [job, ...(old ?? [])],
        );
        select({ kind: "jobs", id: job.id });
      } else {
        const result = await jobRequest<{ taskId: number }>(
          `${url}/instantiate`,
          "POST",
        );
        toast.success("Task created", {
          description: (
            <Link
              to="/task/$taskId"
              params={{ taskId: String(result.taskId) }}
              className="underline"
            >
              Open task #{result.taskId}
            </Link>
          ),
        });
        void client.invalidateQueries({
          queryKey: ["projectDetails", t.projectId],
        });
      }
    },
    onSuccess: (_, kind) => {
      if (kind === "delete") {
        client.setQueryData<TaskTemplate[]>(
          ["task-templates", t.projectId],
          (old) => old?.filter((item) => item.id !== t.id),
        );
        removed();
      }
    },
    onError: (error: Error) => toast.error(error.message),
  });
  const numberOptions = (
    items: { ID: number; Name: string }[] | undefined,
    current: number,
  ) => [
    { value: 0, label: "Choose…" },
    ...(current && !items?.some((i) => i.ID === current)
      ? [{ value: current, label: "Unavailable — choose another" }]
      : []),
    ...(items ?? []).map((i) => ({ value: i.ID, label: i.Name || "Untitled" })),
  ];
  return (
    <PendingFields.Provider value={fields.update}>
      <Card>
        <CardHeader>
          <Button
            variant="ghost"
            size="sm"
            className="mb-3 w-fit -ml-2"
            disabled={edit.isPending || fields.pending || action.isPending}
            onClick={removed}
          >
            <ArrowLeft className="size-4" />
            All templates & jobs
          </Button>
          <CardTitle>Task template</CardTitle>
          <p className="text-sm text-muted-foreground">
            Fields save when you leave them. Changes apply to future tasks only.
          </p>
        </CardHeader>
        <CardContent className="space-y-5">
          <AutoField
            label="Template name"
            value={t.name}
            save={(name) => edit.mutateAsync({ name })}
          />
          <AutoField
            label="Task title"
            value={t.title}
            save={(title) => edit.mutateAsync({ title })}
          />
          <AutoField
            label="Task body (Markdown)"
            value={t.body}
            multiline
            save={(body) => edit.mutateAsync({ body })}
          />
          <div className="grid gap-4 md:grid-cols-2">
            <Choice
              label="Checklist"
              value={t.checklistId}
              options={numberOptions(checklists.data, t.checklistId)}
              disabled={checklists.isPending || edit.isPending}
              change={(v) => edit.mutate({ checklistId: Number(v) })}
            />
            <Choice
              label="Default stage"
              value={t.stageId}
              options={numberOptions(workflow.data?.Stages, t.stageId)}
              disabled={workflow.isPending || edit.isPending}
              change={(v) => edit.mutate({ stageId: Number(v) })}
            />
            <Choice
              label="Task type"
              value={t.typeId}
              options={numberOptions(types.data, t.typeId)}
              disabled={types.isPending || edit.isPending}
              change={(v) => edit.mutate({ typeId: Number(v) })}
            />
            <Choice
              label="Priority"
              value={t.priority}
              options={["Urgent", "High", "Medium", "Low"].map((v) => ({
                value: v,
                label: v,
              }))}
              disabled={edit.isPending}
              change={(priority) => edit.mutate({ priority })}
            />
          </div>
          {(checklists.isError || workflow.error || types.isError) && (
            <p role="alert" className="text-sm text-destructive">
              Some task options failed to load.{" "}
              <Button
                variant="link"
                onClick={() => {
                  void checklists.refetch();
                  void workflow.refetch();
                  void types.refetch();
                }}
              >
                Retry
              </Button>
            </p>
          )}
          <AutoField
            label="Default assignee"
            value={t.assignee}
            save={(assignee) => edit.mutateAsync({ assignee })}
          />
          <AutoField
            label="Additional job instructions"
            value={t.prompt}
            multiline
            save={(prompt) => edit.mutateAsync({ prompt })}
            placeholder="What should the agent check or produce?"
          />
          <p className="text-xs text-muted-foreground">
            Creating a task uses these defaults. A Job assigns Conductor and
            uses the planning, working and review stages in Conductor settings.
          </p>
          <p role="status" className="text-xs text-muted-foreground">
            {edit.isPending
              ? "Saving…"
              : fields.pending
                ? "Unsaved changes. Leave the field to save, or press Escape to discard."
                : "Changes saved automatically"}
          </p>
          <div className="flex flex-wrap gap-2">
            <Button
              disabled={action.isPending || edit.isPending || fields.pending}
              onClick={() => action.mutate("instantiate")}
            >
              <FilePlus2 className="size-4" />
              Create task
            </Button>
            <Button
              variant="outline"
              disabled={action.isPending || edit.isPending || fields.pending}
              onClick={() => action.mutate("job")}
            >
              <CalendarClock className="size-4" />
              Convert to job
            </Button>
            <DeleteEntity
              name={t.name}
              pending={action.isPending || edit.isPending || fields.pending}
              remove={() => action.mutate("delete")}
            />
          </div>
        </CardContent>
      </Card>
    </PendingFields.Provider>
  );
}
function JobEditor({
  job: j,
  template,
  select,
  removed,
}: {
  job: ScheduledJob;
  template?: TaskTemplate;
  select: (selection: Selection) => void;
  removed: () => void;
}) {
  const client = useQueryClient();
  const fields = usePendingFields();
  const edit = useJobEdit<ScheduledJob>(j.projectId, j.id, "jobs");
  const agents = usePersonasQuery();
  const url = `/api/project/${j.projectId}/automation/jobs/${j.id}`;
  const history = useQuery({
    queryKey: ["scheduled-job-runs", j.id],
    queryFn: () => jobRequest<JobRun[]>(`${url}/runs`),
    refetchInterval: 3000,
  });
  const run = useRunJob(j.projectId, j.id);
  const remove = useMutation({
    mutationFn: () => jobRequest<void>(url, "DELETE"),
    onSuccess: () => {
      client.setQueryData<ScheduledJob[]>(
        ["scheduled-jobs", j.projectId],
        (old) => old?.filter((item) => item.id !== j.id),
      );
      removed();
    },
    onError: (error: Error) => toast.error(error.message),
  });
  const active = history.data?.some((r) =>
    ["waiting", "planning", "queued", "working"].includes(r.state),
  );
  return (
    <PendingFields.Provider value={fields.update}>
      <Card>
        <CardHeader>
          <Button
            variant="ghost"
            size="sm"
            className="mb-3 w-fit -ml-2"
            disabled={
              edit.isPending ||
              fields.pending ||
              run.isPending ||
              remove.isPending
            }
            onClick={removed}
          >
            <ArrowLeft className="size-4" />
            All templates & jobs
          </Button>
          <CardTitle>Job</CardTitle>
          <p className="text-sm text-muted-foreground">
            Each run creates a task and keeps its results and branch there for
            review.
          </p>
        </CardHeader>
        <CardContent className="space-y-5">
          <AutoField
            label="Job name"
            value={j.name}
            save={(name) => edit.mutateAsync({ name })}
          />
          <div className="flex items-center gap-2 text-sm">
            <span className="text-muted-foreground">Template:</span>
            <Button
              variant="link"
              className="h-auto p-0"
              disabled={edit.isPending || fields.pending}
              onClick={() => select({ kind: "templates", id: j.templateId })}
            >
              {template?.name || "Open template"}
            </Button>
          </div>
          <Choice
            label="Task agent and model"
            value={j.agentId}
            disabled={edit.isPending || agents.isPending}
            change={(v) => edit.mutate({ agentId: Number(v) })}
            options={[
              { value: 0, label: "Choose an agent…" },
              ...(j.agentId && !agents.data?.some((a) => a.ID === j.agentId)
                ? [
                    {
                      value: j.agentId,
                      label: "Agent unavailable — choose another",
                    },
                  ]
                : []),
              ...(agents.data ?? []).map((a) => ({
                value: a.ID,
                label: `${a.Name || "Untitled agent"} · ${a.Model}`,
              })),
            ]}
          />
          {agents.isError && (
            <p role="alert" className="text-sm text-destructive">
              Could not load agents.{" "}
              <Button variant="link" onClick={() => void agents.refetch()}>
                Retry
              </Button>
            </p>
          )}
          <div className="grid gap-4 md:grid-cols-2">
            <AutoField
              label="Schedule (cron)"
              value={j.schedule}
              placeholder="0 9 * * 1"
              save={(schedule) => edit.mutateAsync({ schedule })}
            />
            <AutoField
              label="Timezone"
              value={j.timezone}
              placeholder="America/Chicago"
              save={(timezone) => edit.mutateAsync({ timezone })}
            />
          </div>
          <p className="text-xs text-muted-foreground">
            Five fields: minute, hour, day, month, weekday. Example:{" "}
            <code>0 9 * * 1</code> runs Mondays at 9:00 in the chosen timezone.
            Leave the schedule blank for manual runs.
          </p>
          <div className="flex flex-wrap items-center gap-3">
            <Button
              variant="outline"
              disabled={edit.isPending || fields.pending}
              onClick={() => edit.mutate({ enabled: !j.enabled })}
            >
              {j.enabled ? "Pause schedule" : "Enable schedule"}
            </Button>
            <span className="text-sm text-muted-foreground">
              {j.enabled && j.nextRun
                ? `Next: ${new Date(j.nextRun * 1000).toLocaleString(undefined, { timeZone: j.timezone, timeZoneName: "short" })}`
                : "Schedule paused · Run now is available"}
            </span>
          </div>
          <p className="text-xs text-muted-foreground">
            Aycorn must be running. After downtime, one missed run is caught up;
            overlapping runs are skipped. Pausing a schedule lets its active run
            finish. Jobs use the project’s Conductor stages and prompts even
            when Conductor is paused.
          </p>
          <Button asChild variant="link" className="h-auto p-0">
            <Link
              to="/project/settings/$projectId"
              params={{ projectId: String(j.projectId) }}
              search={{ tab: "conductor" }}
            >
              Configure stages and Conductor
            </Link>
          </Button>
          {j.lastError && (
            <p role="alert" className="text-sm text-destructive">
              {j.lastError}
            </p>
          )}
          <p role="status" className="text-xs text-muted-foreground">
            {edit.isPending
              ? "Saving…"
              : fields.pending
                ? "Unsaved changes. Leave the field to save, or press Escape to discard."
                : "Changes save automatically"}
          </p>
          <div className="flex flex-wrap gap-2">
            <Button
              disabled={
                run.isPending ||
                edit.isPending ||
                fields.pending ||
                !j.agentId ||
                active
              }
              onClick={() => run.mutate()}
            >
              <Play className="size-4" />
              {run.isPending
                ? "Queuing…"
                : active
                  ? "Run in progress"
                  : "Run now"}
            </Button>
            <DeleteEntity
              name={j.name}
              pending={remove.isPending || !!active || edit.isPending}
              remove={() => remove.mutate()}
            />
          </div>
          <div className="space-y-2 border-t pt-4">
            <h3 className="font-medium">Recent runs</h3>
            {history.isPending ? (
              <p className="text-sm text-muted-foreground">Loading runs…</p>
            ) : history.isError ? (
              <p role="alert" className="text-sm text-destructive">
                Could not load history.{" "}
                <Button variant="link" onClick={() => void history.refetch()}>
                  Retry
                </Button>
              </p>
            ) : history.data?.length === 0 ? (
              <p className="text-sm text-muted-foreground">No runs yet.</p>
            ) : (
              <ul className="divide-y">
                {history.data?.map((r) => (
                  <li key={r.id} className="space-y-1 py-3 text-sm">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      {r.taskId ? (
                        <Link
                          to="/task/$taskId"
                          params={{ taskId: String(r.taskId) }}
                          className="font-medium underline underline-offset-4"
                        >
                          Task #{r.taskId}
                        </Link>
                      ) : (
                        <span>Deleted task</span>
                      )}
                      <span className="text-muted-foreground">
                        {r.state.replaceAll("_", " ")} · {r.trigger}
                      </span>
                    </div>
                    <p className="text-muted-foreground">
                      {new Date(r.createdAt).toLocaleString()}
                    </p>
                    {r.message && <p>{r.message}</p>}
                  </li>
                ))}
              </ul>
            )}
          </div>
        </CardContent>
      </Card>
    </PendingFields.Provider>
  );
}
