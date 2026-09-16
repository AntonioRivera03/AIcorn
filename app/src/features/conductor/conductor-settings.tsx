import { Link } from "@tanstack/react-router";
import { usePersonasQuery } from "@/features/persona/queries/use-personas-query";
import { useId, useState } from "react";
import { AudioLines, CheckCheck, Code2 } from "lucide-react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useProjectWorkflowSettingsQuery } from "@/features/settings/project-workflow/queries/useProjectWorkflowSettingsQuery";
import { useConductor } from "./use-conductor";

function AutoText({
  label,
  value,
  onChange,
  multiline = false,
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  multiline?: boolean;
  placeholder?: string;
}) {
  const id = useId();
  const [draft, setDraft] = useState(value);
  const props = {
    id,
    value: draft,
    placeholder,
    onChange: (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
      setDraft(e.target.value),
    onBlur: () => {
      if (draft !== value) onChange(draft);
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
    </div>
  );
}

const phases = [
  {
    stage: "workingStage",
    prompt: "workingPrompt",
    title: "In progress",
    role: "Assigned agent",
    description:
      "The start tool moves the task here and queues its independent session.",
    icon: Code2,
  },
  {
    stage: "completionStage",
    prompt: "completionPrompt",
    title: "Human review",
    role: "Handoff",
    description:
      "Append a summary and hand the task to you. You review the result and move it to Done.",
    icon: CheckCheck,
  },
] as const;

export function ConductorSettingsTab({ projectId }: { projectId: number }) {
  const conductor = useConductor(projectId);
  const agents = usePersonasQuery();
  const workflow = useProjectWorkflowSettingsQuery(projectId);
  if (conductor.isPending || workflow.isPending)
    return (
      <p className="p-6 text-sm text-muted-foreground">
        Loading Conductor settings…
      </p>
    );
  if (!conductor.data || !workflow.data)
    return (
      <div role="alert" className="p-6 text-destructive">
        Could not load Conductor settings.{" "}
        <Button
          variant="outline"
          onClick={() => {
            void conductor.refetch();
            void workflow.refetch();
          }}
        >
          Retry
        </Button>
      </div>
    );
  const { settings, configurationError } = conductor.data;
  const stages = workflow.data.Stages.filter((s) => s.Type !== "done");
  return (
    <section className="space-y-6 pb-8">
      <Card className="border-conductor/25">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <AudioLines className="size-5 text-conductor" />
            Conductor
          </CardTitle>
          <CardDescription>
            Conductor selects tasks and starts an independent agent session for
            each. The application moves tasks through their configured stages.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex flex-wrap items-center gap-3">
            <Button
              variant={settings.enabled ? "outline" : "default"}
              disabled={
                conductor.update.isPending ||
                (!!configurationError && !settings.enabled)
              }
              onClick={() =>
                conductor.update.mutate({ enabled: !settings.enabled })
              }
            >
              {settings.enabled ? "Pause Conductor" : "Start Conductor"}
            </Button>
            <span className="text-sm text-muted-foreground">
              {settings.enabled
                ? "Managing selected tasks. Pausing lets active sessions finish."
                : "Paused. Send tasks from their actions menu, then start when ready."}
            </span>
          </div>
          {configurationError && (
            <p className="text-sm text-conductor">{configurationError}</p>
          )}
          <AutoText
            key={`selection-${settings.planningPrompt}`}
            label="Task selection instructions"
            value={settings.planningPrompt}
            multiline
            onChange={(value) =>
              conductor.update.mutate({ planningPrompt: value })
            }
          />
          <p className="text-xs text-muted-foreground" role="status">
            {conductor.update.isPending
              ? "Saving…"
              : conductor.update.isError
                ? "That change could not be saved. Check the error and try again."
                : "Changes save automatically. Stage and prompt changes apply to newly started task sessions."}
          </p>
        </CardContent>
      </Card>
      <div className="grid gap-4 lg:grid-cols-2">
        {phases.map((phase, index) => (
          <Card key={phase.stage}>
            <CardHeader>
              <span className="mb-2 flex items-center gap-2 text-xs font-medium uppercase tracking-wider text-conductor">
                <phase.icon className="size-4" />
                {String(index + 1).padStart(2, "0")} · {phase.role}
              </span>
              <CardTitle>{phase.title}</CardTitle>
              <CardDescription>{phase.description}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-5">
              <div className="space-y-2">
                <Label htmlFor={phase.stage}>Workflow stage</Label>
                <Select
                  value={
                    settings[phase.stage]
                      ? String(settings[phase.stage])
                      : "none"
                  }
                  disabled={conductor.update.isPending}
                  onValueChange={(value) =>
                    conductor.update.mutate({
                      [phase.stage]: value === "none" ? 0 : Number(value),
                    })
                  }
                >
                  <SelectTrigger id={phase.stage} className="w-full">
                    <SelectValue placeholder="Choose a stage" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none" disabled={settings.enabled}>
                      Choose a stage
                    </SelectItem>
                    {stages.map((stage) => (
                      <SelectItem
                        key={stage.ID}
                        value={String(stage.ID)}
                        disabled={phases.some(
                          (other) =>
                            other.stage !== phase.stage &&
                            settings[other.stage] === stage.ID,
                        )}
                      >
                        {stage.Name || "Untitled stage"}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <AutoText
                key={`${phase.prompt}-${settings[phase.prompt]}`}
                label={
                  phase.stage === "completionStage"
                    ? "Handoff prompt"
                    : "Stage prompt"
                }
                value={settings[phase.prompt]}
                multiline
                onChange={(value) =>
                  conductor.update.mutate({ [phase.prompt]: value })
                }
              />
            </CardContent>
          </Card>
        ))}
      </div>
      <Card>
        <CardHeader>
          <CardTitle>Agents and execution</CardTitle>
          <CardDescription>
            Conductor is this project’s built-in task dispatcher. It chooses a
            task agent and starts a separate session through MCP. Model choices
            are captured when the task starts.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          <div className="rounded-lg border border-border p-4 space-y-1">
            <p className="font-medium">Conductor · Default orchestrator</p>
            <p className="text-sm text-muted-foreground">
              {agents.data?.find((agent) => agent.BuiltinRole === "conductor")
                ?.Model ?? "Loading model…"}
            </p>
            <p className="text-xs text-muted-foreground">
              Its instructions and workflow skill are bundled with Aycorn.
              Change agent models on the AI page.
            </p>
          </div>
          {agents.isError && (
            <p className="text-sm text-destructive">
              Could not load agent models.{" "}
              <Button variant="link" onClick={() => void agents.refetch()}>
                Retry
              </Button>
            </p>
          )}
          <Button asChild variant="outline">
            <Link to="/personas">Agent models and instructions</Link>
          </Button>
          <div className="flex items-start gap-3">
            <Checkbox
              id="conductor-repository"
              checked={settings.useRepository}
              disabled={conductor.update.isPending}
              onCheckedChange={(value) =>
                conductor.update.mutate({ useRepository: value === true })
              }
            />
            <div className="space-y-1">
              <Label htmlFor="conductor-repository">
                Work in the project repository
              </Label>
              <p className="text-sm text-muted-foreground">
                Uses the repository folder in General settings. Agents edit
                isolated branches; you review and merge their work. Turn this
                off for tasks that produce a written answer.
              </p>
            </div>
          </div>
          <p className="text-xs text-muted-foreground">
            Uses your local Codex login. Each task keeps its own conversation,
            model, fixed instructions and workspace. The task session completes
            its work directly; server code handles the review handoff.
          </p>
        </CardContent>
      </Card>
    </section>
  );
}
