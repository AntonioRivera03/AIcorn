import { Page, PageContent, PageHeader } from "@/components/page/Page";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { ProjectProvider } from "@/contexts/project/ProjectProvider";
import { ProjectDetails } from "@/components/project/project-details";
import { ProjectHeader } from "@/components/project/project-header";
import { ProjectContext } from "@/contexts/project/ProjectContext";
import { useContext, useEffect } from "react";

export const Route = createFileRoute("/project/$projectId")({
  component: RouteComponent,
  validateSearch: (search: Record<string, unknown>): { view?: string } => {
    return {
      view: search.view as string | undefined,
    };
  },
});

function ProjectPageHeader() {
  const { Project } = useContext(ProjectContext);
  return (
    <PageHeader breadcrumb={[{ label: "Projects", to: "/" }, Project.Name]}>
      <ProjectHeader />
    </PageHeader>
  );
}

function ProjectPageContent({
  projectId,
  view,
}: {
  projectId: number;
  view?: string;
}) {
  const navigate = useNavigate({ from: Route.fullPath });
  const { Project } = useContext(ProjectContext);

  useEffect(() => {
    if (!view && Project.DefaultView) {
      navigate({ search: { view: Project.DefaultView }, replace: true });
    }
  }, [view, Project.DefaultView, navigate]);

  const setView = (newView: string) => navigate({ search: { view: newView } });

  return (
    <Page>
      <ProjectPageHeader />
      <PageContent>
        <ProjectDetails
          projectId={projectId}
          view={(view ?? Project.DefaultView) || "list"}
          setView={setView}
        />
      </PageContent>
    </Page>
  );
}

function RouteComponent() {
  const { projectId } = Route.useParams();
  const { view } = Route.useSearch();

  return (
    <ProjectProvider>
      <ProjectPageContent
        projectId={Number.parseInt(projectId)}
        view={view}
      />
    </ProjectProvider>
  );
}
