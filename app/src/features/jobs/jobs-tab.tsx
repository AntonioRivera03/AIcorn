import { useId, useRef, useState } from "react";
import { Link } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { CalendarClock, FilePlus2, Play, Plus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from "@/components/ui/alert-dialog";
import { useProjectWorkflowSettingsQuery } from "@/features/settings/project-workflow/queries/useProjectWorkflowSettingsQuery";
import { usePersonasQuery } from "@/features/persona/queries/use-personas-query";
import type { Checklist, TaskType } from "@/types/types";
import { jobRequest, useJobEdit, useJobResources, type JobRun, type ScheduledJob, type TaskTemplate } from "./use-jobs";

function AutoField({ label, value, save, multiline = false, placeholder }: { label: string; value: string; save: (value: string) => Promise<unknown>; multiline?: boolean; placeholder?: string }) {
  const id = useId();
  const [draft, setDraft] = useState<string | null>(null);
  const [error, setError] = useState("");
  const pending = useRef<string | null>(null);
  async function commit() {
    if (draft === null || (draft === value && !error) || draft === pending.current) return;
    const sent = draft;
    pending.current = sent;
    try { await save(sent); setDraft((current) => current === sent ? null : current); setError(""); }
    catch (e) { setError(e instanceof Error ? e.message : "Could not save. Edit or leave this field to retry."); }
    finally { if (pending.current === sent) pending.current = null; }
  }
  const props = { id, value: draft ?? value, placeholder, onChange: (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => setDraft(e.target.value), onBlur: () => void commit(), onKeyDown: (e: React.KeyboardEvent) => { if (e.key === "Escape") { setDraft(null); setError(""); } } };
  return <div className="space-y-2"><Label htmlFor={id}>{label}</Label>{multiline ? <Textarea {...props} className="min-h-28 resize-y" /> : <Input {...props} />}{error && <p role="alert" className="text-sm text-destructive">{error}</p>}</div>;
}
function Choice({ label, value, options, change, disabled }: { label: string; value: string | number; options: { value: string | number; label: string }[]; change: (value: string) => void; disabled?: boolean }) {
  const id = useId();
  return <div className="space-y-2"><Label htmlFor={id}>{label}</Label><select id={id} value={value} disabled={disabled} onChange={(e) => change(e.target.value)} className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50">{options.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}</select></div>;
}
function DeleteEntity({ name, pending, remove }: { name: string; pending: boolean; remove: () => void }) {
  return <AlertDialog><AlertDialogTrigger asChild><Button variant="outline" size="sm" disabled={pending}><Trash2 className="size-4" />Delete</Button></AlertDialogTrigger><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Delete {name || "this item"}?</AlertDialogTitle><AlertDialogDescription>This cannot be undone. Tasks already created from it remain on the board. Jobs using a template must be deleted first; active jobs cannot be deleted.</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>Cancel</AlertDialogCancel><AlertDialogAction onClick={remove}>Delete</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>;
}

type Selection = { kind: "templates" | "jobs"; id: number };
export function JobsTab({ projectId }: { projectId: number }) {
  const client = useQueryClient();
  const { templates, jobs } = useJobResources(projectId);
  const [selection, setSelection] = useState<Selection | null>(null);
  const create = useMutation({
    mutationFn: () => jobRequest<TaskTemplate>(`/api/project/${projectId}/automation/templates`, "POST"),
    onSuccess: (t) => { client.setQueryData<TaskTemplate[]>(["task-templates", projectId], (old) => [t, ...(old ?? [])]); setSelection({ kind: "templates", id: t.id }); },
    onError: (error: Error) => toast.error(error.message),
  });
  if (templates.isPending || jobs.isPending) return <p className="p-6 text-sm text-muted-foreground">Loading templates and jobs…</p>;
  if (!templates.data || !jobs.data) return <div role="alert" className="p-6 text-destructive">Could not load jobs. <Button variant="outline" onClick={() => { void templates.refetch(); void jobs.refetch(); }}>Retry</Button></div>;
  const selected = selection ?? (jobs.data[0] ? { kind: "jobs", id: jobs.data[0].id } : templates.data[0] ? { kind: "templates", id: templates.data[0].id } : null);
  const template = selected?.kind === "templates" ? templates.data.find((t) => t.id === selected.id) : undefined;
  const job = selected?.kind === "jobs" ? jobs.data.find((j) => j.id === selected.id) : undefined;
  return <section className="space-y-5 pb-8">
    <div className="flex flex-wrap items-start justify-between gap-4"><div><h2 className="text-lg font-semibold">Jobs & templates</h2><p className="max-w-2xl text-sm text-muted-foreground">Create reusable tasks, then run them yourself or assign an agent and a schedule. Jobs run even when the board’s Conductor is paused.</p></div><Button onClick={() => create.mutate()} disabled={create.isPending}><Plus className="size-4" />New template</Button></div>
    <div className="grid items-start gap-5 lg:grid-cols-[16rem_1fr]">
      <nav aria-label="Jobs and templates" className="space-y-5 rounded-lg border p-3">
        {([{ kind: "jobs", label: "Jobs", items: jobs.data }, { kind: "templates", label: "Templates", items: templates.data }] as const).map((group) => <div key={group.kind}><h3 className="px-2 pb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">{group.label}</h3>{group.items.length === 0 && <p className="px-2 text-sm text-muted-foreground">None yet</p>}{group.items.map((item) => <button key={item.id} aria-current={selected?.kind === group.kind && selected.id === item.id ? "true" : undefined} onClick={() => setSelection({ kind: group.kind, id: item.id })} className="flex w-full items-center gap-2 rounded-md px-2 py-2 text-left text-sm hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring aria-[current=true]:bg-accent aria-[current=true]:font-medium">{group.kind === "jobs" ? <CalendarClock className="size-4 shrink-0" /> : <FilePlus2 className="size-4 shrink-0" />}<span className="truncate">{item.name || "Untitled"}</span></button>)}</div>)}
      </nav>
      {template ? <TemplateEditor key={`template-${template.id}`} template={template} select={setSelection} removed={() => setSelection(null)} /> : job ? <JobEditor key={`job-${job.id}`} job={job} template={templates.data.find((t) => t.id === job.templateId)} select={setSelection} removed={() => setSelection(null)} /> : <div className="rounded-lg border border-dashed p-10 text-center text-sm text-muted-foreground">Start with a template, such as a weekly repository audit. Each run creates a fresh task.</div>}
    </div>
  </section>;
}
function TemplateEditor({ template: t, select, removed }: { template: TaskTemplate; select: (selection: Selection) => void; removed: () => void }) {
  const client = useQueryClient();
  const edit = useJobEdit<TaskTemplate>(t.projectId, t.id, "templates");
  const workflow = useProjectWorkflowSettingsQuery(t.projectId);
  const checklists = useQuery({ queryKey: ["job-checklists", t.projectId], queryFn: () => jobRequest<Checklist[]>(`/api/project/checklist/${t.projectId}`) });
  const types = useQuery({ queryKey: ["projectTaskTypes", t.projectId], queryFn: () => jobRequest<TaskType[]>(`/api/project/${t.projectId}/settings/task-types/enabled`) });
  const action = useMutation({
    scope: { id: `templates-${t.projectId}-${t.id}` },
    mutationFn: async (kind: "instantiate" | "job" | "delete") => {
      const url = `/api/project/${t.projectId}/automation/templates/${t.id}`;
      if (kind === "delete") { await jobRequest<void>(url, "DELETE"); return; }
      if (kind === "job") {
        const job = await jobRequest<ScheduledJob>(`${url}/job`, "POST");
        client.setQueryData<ScheduledJob[]>(["scheduled-jobs", t.projectId], (old) => [job, ...(old ?? [])]);
        select({ kind: "jobs", id: job.id });
      } else {
        const result = await jobRequest<{ taskId: number }>(`${url}/instantiate`, "POST");
        toast.success("Task created", { description: <Link to="/task/$taskId" params={{ taskId: String(result.taskId) }} className="underline">Open task #{result.taskId}</Link> });
        void client.invalidateQueries({ queryKey: ["projectDetails", t.projectId] });
      }
    },
    onSuccess: (_, kind) => { if (kind === "delete") { client.setQueryData<TaskTemplate[]>(["task-templates", t.projectId], (old) => old?.filter((item) => item.id !== t.id)); removed(); } },
    onError: (error: Error) => toast.error(error.message),
  });
  const numberOptions = (items: { ID: number; Name: string }[] | undefined, current: number) => [{ value: 0, label: "Choose…" }, ...(current && !items?.some((i) => i.ID === current) ? [{ value: current, label: "Unavailable — choose another" }] : []), ...(items ?? []).map((i) => ({ value: i.ID, label: i.Name || "Untitled" }))];
  return <Card><CardHeader><CardTitle>Task template</CardTitle><p className="text-sm text-muted-foreground">Fields save when you leave them. Changes apply to future tasks only.</p></CardHeader><CardContent className="space-y-5">
    <AutoField label="Template name" value={t.name} save={(name) => edit.mutateAsync({ name })} />
    <AutoField label="Task title" value={t.title} save={(title) => edit.mutateAsync({ title })} />
    <AutoField label="Task body (Markdown)" value={t.body} multiline save={(body) => edit.mutateAsync({ body })} />
    <div className="grid gap-4 md:grid-cols-2">
      <Choice label="Checklist" value={t.checklistId} options={numberOptions(checklists.data, t.checklistId)} disabled={checklists.isPending || edit.isPending} change={(v) => edit.mutate({ checklistId: Number(v) })} />
      <Choice label="Default stage" value={t.stageId} options={numberOptions(workflow.data?.Stages, t.stageId)} disabled={workflow.isPending || edit.isPending} change={(v) => edit.mutate({ stageId: Number(v) })} />
      <Choice label="Task type" value={t.typeId} options={numberOptions(types.data, t.typeId)} disabled={types.isPending || edit.isPending} change={(v) => edit.mutate({ typeId: Number(v) })} />
      <Choice label="Priority" value={t.priority} options={["Urgent", "High", "Medium", "Low"].map((v) => ({ value: v, label: v }))} disabled={edit.isPending} change={(priority) => edit.mutate({ priority })} />
    </div>
    {(checklists.isError || workflow.error || types.isError) && <p role="alert" className="text-sm text-destructive">Some task options failed to load. <Button variant="link" onClick={() => { void checklists.refetch(); void workflow.refetch(); void types.refetch(); }}>Retry</Button></p>}
    <AutoField label="Default assignee" value={t.assignee} save={(assignee) => edit.mutateAsync({ assignee })} />
    <AutoField label="Additional job instructions" value={t.prompt} multiline save={(prompt) => edit.mutateAsync({ prompt })} placeholder="What should the agent check or produce?" />
    <p className="text-xs text-muted-foreground">Creating a task uses these defaults. A Job assigns Conductor and uses the planning, working and review stages in Conductor settings.</p>
    <p role="status" className="text-xs text-muted-foreground">{edit.isPending ? "Saving…" : edit.isError ? "A change needs attention. Check the field error." : "Changes saved automatically"}</p>
    <div className="flex flex-wrap gap-2"><Button disabled={action.isPending || edit.isPending || edit.isError} onClick={() => action.mutate("instantiate")}><FilePlus2 className="size-4" />Create task</Button><Button variant="outline" disabled={action.isPending || edit.isPending || edit.isError} onClick={() => action.mutate("job")}><CalendarClock className="size-4" />Convert to job</Button><DeleteEntity name={t.name} pending={action.isPending || edit.isPending || edit.isError} remove={() => action.mutate("delete")} /></div>
  </CardContent></Card>;
}
function JobEditor({ job: j, template, select, removed }: { job: ScheduledJob; template?: TaskTemplate; select: (selection: Selection) => void; removed: () => void }) {
  const client = useQueryClient();
  const edit = useJobEdit<ScheduledJob>(j.projectId, j.id, "jobs");
  const agents = usePersonasQuery();
  const url = `/api/project/${j.projectId}/automation/jobs/${j.id}`;
  const history = useQuery({ queryKey: ["scheduled-job-runs", j.id], queryFn: () => jobRequest<JobRun[]>(`${url}/runs`), refetchInterval: 3000 });
  const key = useRef<string | null>(null);
  const run = useMutation({
    mutationFn: () => { key.current ??= crypto.randomUUID(); return jobRequest<{ taskId: number }>(`${url}/run`, "POST", { key: key.current }); },
    onSuccess: (result) => {
      key.current = null;
      toast.success("Job queued", { description: <Link to="/task/$taskId" params={{ taskId: String(result.taskId) }} className="underline">Open task #{result.taskId}</Link> });
      void history.refetch(); void client.invalidateQueries({ queryKey: ["scheduled-jobs", j.projectId] }); void client.invalidateQueries({ queryKey: ["projectDetails", j.projectId] }); void client.invalidateQueries({ queryKey: ["conductor", j.projectId] });
    },
    onError: (error: Error) => toast.error(error.message),
  });
  const remove = useMutation({ mutationFn: () => jobRequest<void>(url, "DELETE"), onSuccess: () => { client.setQueryData<ScheduledJob[]>(["scheduled-jobs", j.projectId], (old) => old?.filter((item) => item.id !== j.id)); removed(); }, onError: (error: Error) => toast.error(error.message) });
  const active = history.data?.some((r) => ["waiting", "planning", "queued", "working"].includes(r.state));
  return <Card><CardHeader><CardTitle>Job</CardTitle><p className="text-sm text-muted-foreground">Each run creates a task and keeps its results and branch there for review.</p></CardHeader><CardContent className="space-y-5">
    <AutoField label="Job name" value={j.name} save={(name) => edit.mutateAsync({ name })} />
    <div className="flex items-center gap-2 text-sm"><span className="text-muted-foreground">Template:</span><Button variant="link" className="h-auto p-0" onClick={() => select({ kind: "templates", id: j.templateId })}>{template?.name || "Open template"}</Button></div>
    <Choice label="Task agent and model" value={j.agentId} disabled={edit.isPending || agents.isPending} change={(v) => edit.mutate({ agentId: Number(v) })} options={[{ value: 0, label: "Choose an agent…" }, ...(j.agentId && !agents.data?.some((a) => a.ID === j.agentId) ? [{ value: j.agentId, label: "Agent unavailable — choose another" }] : []), ...(agents.data ?? []).map((a) => ({ value: a.ID, label: `${a.Name || "Untitled agent"} · ${a.Model}` }))]} />
    {agents.isError && <p role="alert" className="text-sm text-destructive">Could not load agents. <Button variant="link" onClick={() => void agents.refetch()}>Retry</Button></p>}
    <div className="grid gap-4 md:grid-cols-2"><AutoField label="Schedule (cron)" value={j.schedule} placeholder="0 9 * * 1" save={(schedule) => edit.mutateAsync({ schedule })} /><AutoField label="Timezone" value={j.timezone} placeholder="America/Chicago" save={(timezone) => edit.mutateAsync({ timezone })} /></div>
    <p className="text-xs text-muted-foreground">Five fields: minute, hour, day, month, weekday. Example: <code>0 9 * * 1</code> runs Mondays at 9:00 in the chosen timezone. Leave the schedule blank for manual runs.</p>
    <div className="flex flex-wrap items-center gap-3"><Button variant="outline" disabled={edit.isPending || edit.isError} onClick={() => edit.mutate({ enabled: !j.enabled })}>{j.enabled ? "Pause schedule" : "Enable schedule"}</Button><span className="text-sm text-muted-foreground">{j.enabled && j.nextRun ? `Next: ${new Date(j.nextRun * 1000).toLocaleString(undefined, { timeZone: j.timezone, timeZoneName: "short" })}` : "Schedule paused · Run now is available"}</span></div>
    <p className="text-xs text-muted-foreground">Aycorn must be running. After downtime, one missed run is caught up; overlapping runs are skipped. Pausing a schedule lets its active run finish. Jobs use the project’s Conductor stages and prompts even when Conductor is paused.</p>
    <Button asChild variant="link" className="h-auto p-0"><Link to="/project/settings/$projectId" params={{ projectId: String(j.projectId) }} search={{ tab: "conductor" }}>Configure stages and Conductor</Link></Button>
    {j.lastError && <p role="alert" className="text-sm text-destructive">{j.lastError}</p>}
    <p role="status" className="text-xs text-muted-foreground">{edit.isPending ? "Saving…" : edit.isError ? "A change was not saved. Correct the field before running." : "Changes save automatically"}</p>
    <div className="flex flex-wrap gap-2"><Button disabled={run.isPending || edit.isPending || edit.isError || !j.agentId || active} onClick={() => run.mutate()}><Play className="size-4" />{run.isPending ? "Queuing…" : active ? "Run in progress" : "Run now"}</Button><DeleteEntity name={j.name} pending={remove.isPending || !!active || edit.isPending} remove={() => remove.mutate()} /></div>
    <div className="space-y-2 border-t pt-4"><h3 className="font-medium">Recent runs</h3>{history.isPending ? <p className="text-sm text-muted-foreground">Loading runs…</p> : history.isError ? <p role="alert" className="text-sm text-destructive">Could not load history. <Button variant="link" onClick={() => void history.refetch()}>Retry</Button></p> : history.data?.length === 0 ? <p className="text-sm text-muted-foreground">No runs yet.</p> : <ul className="divide-y">{history.data?.map((r) => <li key={r.id} className="space-y-1 py-3 text-sm"><div className="flex flex-wrap items-center justify-between gap-2">{r.taskId ? <Link to="/task/$taskId" params={{ taskId: String(r.taskId) }} className="font-medium underline underline-offset-4">Task #{r.taskId}</Link> : <span>Deleted task</span>}<span className="text-muted-foreground">{r.state.replaceAll("_", " ")} · {r.trigger}</span></div><p className="text-muted-foreground">{new Date(r.createdAt).toLocaleString()}</p>{r.message && <p>{r.message}</p>}</li>)}</ul>}</div>
  </CardContent></Card>;
}
