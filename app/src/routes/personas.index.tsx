import { AIEngineSettings } from "@/features/ai/ai-engine-settings";
import { createFileRoute } from "@tanstack/react-router";
import {
  Page,
  PageContent,
  PageHeader,
  PageTitle,
} from "@/components/page/Page";
import { PersonasPage } from "@/features/persona/personas-page";

export const Route = createFileRoute("/personas/")({
  component: RouteComponent,
});

function RouteComponent() {
  return (
    <Page>
      <PageHeader breadcrumb={["AI"]} />
      <PageContent>
        <PageTitle
          title="AI"
          description="Choose the models for Aycorn’s agents. Roles, instructions and skills are managed by the app."
        />
        <AIEngineSettings />
        <h2 className="mt-6 mb-3 font-medium">Agents</h2>
        <PersonasPage />
      </PageContent>
    </Page>
  );
}
