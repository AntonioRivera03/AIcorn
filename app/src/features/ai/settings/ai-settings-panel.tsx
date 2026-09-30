import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { AdvancedSection } from "@/features/ai/settings/advanced-section";
import { AgentsSection } from "@/features/ai/settings/agents-section";
import { HarnessSection } from "@/features/ai/settings/harness-section";
import { useAISettings } from "@/features/ai/queries/use-ai";

export function AISettingsPanel() {
  const query = useAISettings();

  if (query.isError)
    return (
      <p role="alert" className="text-sm text-destructive">
        {query.error.message || "Could not load AI settings."}{" "}
        <Button variant="link" onClick={() => void query.refetch()}>
          Retry
        </Button>
      </p>
    );
  if (!query.data)
    return <p className="text-sm text-muted-foreground">Loading AI settings…</p>;

  return (
    <div className="flex max-w-3xl flex-col gap-8">
      <HarnessSection data={query.data} checking={query.isFetching} />
      <Separator />
      <AgentsSection settings={query.data.settings} />
      <AdvancedSection data={query.data} />
    </div>
  );
}
