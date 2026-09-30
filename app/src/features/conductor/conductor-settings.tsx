import { Link } from "@tanstack/react-router";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { InstructionsField } from "@/features/conductor/conductor-settings/instructions-field";
import { StageSelect } from "@/features/conductor/conductor-settings/stage-select";
import { useConductor } from "@/features/conductor/use-conductor";
import { SectionHeading } from "@/features/settings/section-heading";
import { useProjectWorkflowSettingsQuery } from "@/features/settings/project-workflow/queries/useProjectWorkflowSettingsQuery";

// Conductor is started and paused from the project board; this tab only
// holds what it needs to run well in this project.
export function ConductorSettingsTab({ projectId }: { projectId: number }) {
  const conductor = useConductor(projectId);
  const workflow = useProjectWorkflowSettingsQuery(projectId);

  if (conductor.isPending || workflow.isPending)
    return (
      <p className="py-6 text-sm text-muted-foreground">
        Loading Conductor settings…
      </p>
    );
  if (!conductor.data || !workflow.data)
    return (
      <p role="alert" className="py-6 text-sm text-destructive">
        Could not load Conductor settings.{" "}
        <Button
          variant="link"
          onClick={() => {
            void conductor.refetch();
            void workflow.refetch();
          }}
        >
          Retry
        </Button>
      </p>
    );

  const { settings, configurationError } = conductor.data;
  // Handing off to a human means the task isn't finished, so Done stages
  // can't hold either role.
  const stages = workflow.data.Stages.filter((stage) => stage.Type !== "done");
  const saving = conductor.update.isPending;
  const save = conductor.update.mutate;

  return (
    <div className="flex max-w-3xl flex-col gap-8 pt-2 pb-8">
      <div className="flex flex-col gap-1">
        <h2 className="text-lg font-medium">Conductor</h2>
        <p className="max-w-prose text-sm text-muted-foreground">
          Conductor picks up the tasks you send it and starts the right agent on
          each one.
        </p>
      </div>

      <section className="flex flex-col gap-4">
        <SectionHeading
          title="Stages"
          description="Where Conductor moves a task on this project's workflow."
        />
        <div className="grid gap-4 sm:grid-cols-2">
          <StageSelect
            id="conductor-working-stage"
            label="While an agent works"
            hint="The task moves here when its agent starts."
            value={settings.workingStage}
            otherValue={settings.completionStage}
            stages={stages}
            required={settings.enabled}
            disabled={saving}
            onChange={(workingStage) => save({ workingStage })}
          />
          <StageSelect
            id="conductor-finished-stage"
            label="When the agent finishes"
            hint="The agent adds a summary of its work and the task moves here for you. Conductor is done with it."
            value={settings.completionStage}
            otherValue={settings.workingStage}
            stages={stages}
            required={settings.enabled}
            disabled={saving}
            onChange={(completionStage) => save({ completionStage })}
          />
        </div>
        {/* Until both stages are picked, the hints already say what's needed. */}
        {configurationError && settings.workingStage > 0 && settings.completionStage > 0 && (
          <p role="status" className="text-sm text-muted-foreground">
            {configurationError}
          </p>
        )}
      </section>

      <Separator />

      <section className="flex items-start gap-3">
        <Checkbox
          id="conductor-repository"
          checked={settings.useRepository}
          disabled={saving}
          onCheckedChange={(value) => save({ useRepository: value === true })}
        />
        <div className="flex flex-col gap-1">
          <Label htmlFor="conductor-repository">
            Work in the project repository
          </Label>
          <p className="text-sm text-muted-foreground">
            Agents work on their own branch of the repository linked in
            General, and you merge the results. Turn this off when tasks only
            need a written answer.
          </p>
        </div>
      </section>

      <InstructionsField
        key={settings.planningPrompt}
        value={settings.planningPrompt}
        onSave={(planningPrompt) => save({ planningPrompt })}
      />

      <p className="text-sm text-muted-foreground">
        Conductor uses the workspace's default model. Agents and their models
        are in{" "}
        <Link
          to="/app/settings"
          search={{ tab: "ai" }}
          className="text-foreground underline-offset-4 hover:underline"
        >
          Aycorn AI settings
        </Link>
        .
      </p>
    </div>
  );
}
