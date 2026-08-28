import { createFileRoute } from "@tanstack/react-router";
import { Page, PageContent, PageHeader, PageTitle } from "@/components/page/Page";
import { PersonasPage } from "@/features/persona/personas-page";

export const Route = createFileRoute("/personas/")({
  component: RouteComponent,
});

function RouteComponent() {
  return (
    <Page>
      <PageHeader breadcrumb={["Personas"]} />
      <PageContent>
        <PageTitle
          title="Personas"
          description="Configure reusable AI identities, models, prompts, and tool access."
        />
        <PersonasPage />
      </PageContent>
    </Page>
  );
}
