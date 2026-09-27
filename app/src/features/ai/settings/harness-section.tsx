import { toast } from "sonner";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ModelSelect } from "@/features/ai/settings/model-select";
import {
  useHarnessModels,
  useUpdateAISettings,
} from "@/features/ai/queries/use-ai";
import type { AISettingsResponse } from "@/features/ai/types";
import { SectionHeading } from "@/features/settings/section-heading";
import type { PersonaHarness } from "@/types/types";

type HarnessSectionProps = {
  data: AISettingsResponse;
  checking: boolean;
};

export function HarnessSection({ data, checking }: HarnessSectionProps) {
  const { settings, providers, engine } = data;
  const update = useUpdateAISettings();
  const models = useHarnessModels(settings.harness);
  const current = providers.find((p) => p.id === settings.harness);

  const changeHarness = (harness: PersonaHarness) => {
    if (harness === settings.harness) return;
    // An empty model tells the server to start the new harness on its
    // default; agent models reset to "Default" in the same save.
    update.mutate(
      { ...settings, harness, model: "" },
      {
        onSuccess: () => {
          const name = providers.find((p) => p.id === harness)?.name;
          toast.success(`Switched to ${name}. Agents now use its default model.`);
        },
      },
    );
  };

  const changeModel = (model: string) => {
    if (model === settings.model) return;
    update.mutate({ ...settings, model });
  };

  return (
    <section className="flex flex-col gap-4">
      <SectionHeading
        title="Harness"
        description="The coding agent that runs every AI session in this workspace, using the server's own login."
      />
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="flex flex-col gap-2">
          <Label htmlFor="ai-harness">Harness</Label>
          <Select
            value={settings.harness}
            onValueChange={(value) => changeHarness(value as PersonaHarness)}
            disabled={update.isPending}
          >
            <SelectTrigger id="ai-harness" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {providers.map((provider) => (
                <SelectItem key={provider.id} value={provider.id}>
                  {provider.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="ai-model">Default model</Label>
          <ModelSelect
            id="ai-model"
            className="w-full"
            harness={settings.harness}
            value={settings.model}
            onChange={changeModel}
            disabled={update.isPending}
          />
        </div>
      </div>
      <p role="status" className="text-sm text-muted-foreground">
        {checking
          ? `Checking ${current?.name ?? "the harness"}…`
          : engine.ready
            ? `Ready · ${engine.version}.`
            : engine.error}{" "}
        {models.data &&
          (models.data.live
            ? "Models come from the server's Codex login."
            : "Models come from Aycorn's built-in list.")}
      </p>
      <p className="text-xs text-muted-foreground">
        Conductor, project chats, and any agent set to Default use the default
        model.
      </p>
    </section>
  );
}
