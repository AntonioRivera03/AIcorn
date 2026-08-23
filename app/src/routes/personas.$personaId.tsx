import { createFileRoute } from "@tanstack/react-router";
import { Page, PageContent, PageHeader } from "@/components/page/Page";
import { PersonaEditorPage } from "@/features/persona/persona-editor-page";

type PersonaSearch = {
  new?: boolean;
};

export const Route = createFileRoute("/personas/$personaId")({
  validateSearch: (search: Record<string, unknown>): PersonaSearch => ({
    new: search.new === true || search.new === "true" ? true : undefined,
  }),
  component: RouteComponent,
});

function RouteComponent() {
  const { personaId } = Route.useParams();
  const search = Route.useSearch();

  return (
    <Page>
      <PageHeader
        breadcrumb={[
          { label: "Personas", to: "/personas" },
          "Persona Editor",
        ]}
      />
      <PageContent>
        <PersonaEditorPage personaId={Number(personaId)} autoFocusName={search.new === true} />
      </PageContent>
    </Page>
  );
}
