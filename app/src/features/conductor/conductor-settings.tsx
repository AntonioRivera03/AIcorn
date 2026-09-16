import { Link } from "@tanstack/react-router";
import { usePersonasQuery } from "@/features/persona/queries/use-personas-query";
import { useId, useState } from "react";
import { AudioLines, CheckCheck, ClipboardList, Code2 } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useProjectWorkflowSettingsQuery } from "@/features/settings/project-workflow/queries/useProjectWorkflowSettingsQuery";
import { useConductor } from "./use-conductor";

function AutoText({ label, value, onChange, multiline = false, placeholder }: { label: string; value: string; onChange: (value: string) => void; multiline?: boolean; placeholder?: string }) {
  const id = useId();
  const [draft, setDraft] = useState(value);
  const props = { id, value: draft, placeholder, onChange: (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => setDraft(e.target.value), onBlur: () => { if (draft !== value) onChange(draft); } };
  return <div className="space-y-2"><Label htmlFor={id}>{label}</Label>{multiline ? <Textarea {...props} className="min-h-28 resize-y" /> : <Input {...props} />}</div>;
}

const phases = [
  { stage: "planningStage", prompt: "planningPrompt", title: "Planning", role: "Conductor", description: "Check readiness, add context, and flag missing information. Validated tasks wait here until an agent starts.", icon: ClipboardList },
  { stage: "workingStage", prompt: "workingPrompt", title: "In progress", role: "Assigned agent", description: "The system moves the task here when its agent starts working, after it leaves the queue.", icon: Code2 },
  { stage: "completionStage", prompt: "completionPrompt", title: "Human review", role: "Handoff", description: "Append a summary and hand the task to you. You review the result and move it to Done.", icon: CheckCheck },
] as const;

export function ConductorSettingsTab({ projectId }: { projectId: number }) {
  const conductor = useConductor(projectId);
  const agents = usePersonasQuery();
  const workflow = useProjectWorkflowSettingsQuery(projectId);
  if (conductor.isPending || workflow.isPending) return <p className="p-6 text-sm text-muted-foreground">Loading Conductor settings…</p>;
  if (!conductor.data || !workflow.data) return <div role="alert" className="p-6 text-destructive">Could not load Conductor settings. <Button variant="outline" onClick={() => { void conductor.refetch(); void workflow.refetch(); }}>Retry</Button></div>;
  const { settings, configurationError } = conductor.data;
  const stages = workflow.data.Stages.filter((s) => s.Type !== "done");
  return <section className="space-y-6 pb-8">
    <Card className="border-conductor/25">
      <CardHeader>
        <CardTitle className="flex items-center gap-2"><AudioLines className="size-5 text-conductor" />Conductor</CardTitle>
        <CardDescription>Delegate selected tasks on this board. Conductor coordinates planning, research, coding and review, and handles ticket stages.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex flex-wrap items-center gap-3">
          <Button variant={settings.enabled ? "outline" : "default"} disabled={conductor.update.isPending || (!!configurationError && !settings.enabled)} onClick={() => conductor.update.mutate({ enabled: !settings.enabled })}>{settings.enabled ? "Pause Conductor" : "Start Conductor"}</Button>
          <span className="text-sm text-muted-foreground">{settings.enabled ? "Managing selected tasks. Pausing lets active sessions finish." : "Paused. Send tasks from their actions menu, then start when ready."}</span>
        </div>
        {configurationError && <p className="text-sm text-conductor">{configurationError}</p>}
        <p className="text-xs text-muted-foreground" role="status">{conductor.update.isPending ? "Saving…" : conductor.update.isError ? "That change could not be saved. Check the error and try again." : "Changes save automatically. Stage and prompt changes apply to the next planning cycle."}</p>
      </CardContent>
    </Card>
    <div className="grid gap-4 lg:grid-cols-3">
      {phases.map((phase, index) => <Card key={phase.stage}>
        <CardHeader>
          <span className="mb-2 flex items-center gap-2 text-xs font-medium uppercase tracking-wider text-conductor"><phase.icon className="size-4" />{String(index + 1).padStart(2, "0")} · {phase.role}</span>
          <CardTitle>{phase.title}</CardTitle><CardDescription>{phase.description}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          <div className="space-y-2">
            <Label htmlFor={phase.stage}>Workflow stage</Label>
            <Select value={settings[phase.stage] ? String(settings[phase.stage]) : "none"} disabled={conductor.update.isPending} onValueChange={(value) => conductor.update.mutate({ [phase.stage]: value === "none" ? 0 : Number(value) })}>
              <SelectTrigger id={phase.stage} className="w-full"><SelectValue placeholder="Choose a stage" /></SelectTrigger>
              <SelectContent><SelectItem value="none" disabled={settings.enabled}>Choose a stage</SelectItem>{stages.map((stage) => <SelectItem key={stage.ID} value={String(stage.ID)} disabled={phases.some((other) => other.stage !== phase.stage && settings[other.stage] === stage.ID)}>{stage.Name || "Untitled stage"}</SelectItem>)}</SelectContent>
            </Select>
          </div>
          <AutoText key={`${phase.prompt}-${settings[phase.prompt]}`} label={phase.stage === "completionStage" ? "Handoff prompt" : "Stage prompt"} value={settings[phase.prompt]} multiline onChange={(value) => conductor.update.mutate({ [phase.prompt]: value })} />
        </CardContent>
      </Card>)}
    </div>
    <Card>
      <CardHeader><CardTitle>Agents and execution</CardTitle><CardDescription>Create your agents on the AI page, then choose who plans and who carries out your tasks. Their model and instructions are captured when planning starts.</CardDescription></CardHeader>
      <CardContent className="space-y-5">
        <div className="grid gap-5 md:grid-cols-2">
          {([{ key: "conductorAgentId", label: "Conductor agent" }, { key: "taskAgentId", label: "Task agent" }] as const).map(({key, label}) => <div className="space-y-2" key={key}>
            <Label htmlFor={key}>{label}</Label>
            <Select value={settings[key] ? String(settings[key]) : "none"} disabled={conductor.update.isPending || agents.isPending} onValueChange={(value) => conductor.update.mutate({ [key]: value === "none" ? 0 : Number(value) })}>
              <SelectTrigger id={key} className="w-full"><SelectValue placeholder="Choose an agent" /></SelectTrigger>
              <SelectContent>
                <SelectItem value="none" disabled={settings.enabled}>Choose an agent</SelectItem>
                {settings[key] > 0 && !agents.data?.some((a) => a.ID === settings[key]) && <SelectItem value={String(settings[key])} disabled>Agent unavailable · choose another</SelectItem>}
                {agents.data?.map((agent) => <SelectItem key={agent.ID} value={String(agent.ID)}>{agent.Name || "Untitled agent"} · {agent.Model}</SelectItem>)}
              </SelectContent>
            </Select>
          </div>)}
        </div>
        {agents.isError && <p className="text-sm text-destructive">Could not load agents. <Button variant="link" onClick={() => void agents.refetch()}>Retry</Button></p>}
        <Button asChild variant="outline"><Link to="/personas">Manage custom agents</Link></Button>
        <div className="flex items-start gap-3">
          <Checkbox id="conductor-repository" checked={settings.useRepository} disabled={conductor.update.isPending} onCheckedChange={(value) => conductor.update.mutate({ useRepository: value === true })} />
          <div className="space-y-1"><Label htmlFor="conductor-repository">Work in the project repository</Label><p className="text-sm text-muted-foreground">Uses the repository folder in General settings. Agents edit isolated branches; you review and merge their work. Turn this off for tasks that produce a written answer.</p></div>
        </div>
        <p className="text-xs text-muted-foreground">Uses your local Codex login. Conductor owns each ticket and can delegate to up to four subagents. The task agent supplies the coder’s model and instructions. Planning is read-only; implementation runs in an isolated worktree. Command network access is disabled.</p>
      </CardContent>
    </Card>
  </section>;
}
