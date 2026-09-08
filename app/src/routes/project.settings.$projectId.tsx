import {
  Page,
  PageContent,
  PageHeader,
  PageTitle,
} from "@/components/page/Page";
import { Separator } from "@/components/ui/separator";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { EmptyPanel } from "@/features/settings/empty-panel";
import { useProjectWorkflowSettingsQuery } from "@/features/settings/project-workflow/queries/useProjectWorkflowSettingsQuery";
import { ProjectWorkflowTab } from "@/features/settings/project-workflow/project-workflow-tab";
import { ProjectTaskTypesTab } from "@/features/settings/project-task-types/project-task-types-tab";
import { ProjectGeneralTab } from "@/features/settings/project-general/project-general-tab";
import { ConductorSettingsTab } from "@/features/conductor/conductor-settings";
import { createFileRoute } from "@tanstack/react-router";
import {
  LandPlotIcon,
  AudioLines,
  Settings2Icon,
  TagsIcon,
  WorkflowIcon,
} from "lucide-react";

export const Route = createFileRoute("/project/settings/$projectId")({
  validateSearch: (search: Record<string, unknown>) => ({
    tab: (search.tab as string | undefined) ?? "general",
  }),
  component: RouteComponent,
});

function RouteComponent() {
  const { projectId } = Route.useParams();
  const { tab } = Route.useSearch();
  const id = Number.parseInt(projectId, 10);
  const { data } = useProjectWorkflowSettingsQuery(id);
  const projectName = data?.Project?.Name ?? "";

  return (
    <Page>
      <PageHeader
        breadcrumb={[
          { label: "Projects", to: "/" },
          {
            label: projectName !== "" ? projectName : "New Project",
            to: "/project/$projectId",
            params: { projectId },
          },
          { label: "Settings" },
        ]}
      />
      <PageContent>
        <PageTitle
          title="Project Settings"
          description="Manage your project's behavior and settings."
        />

        <Tabs defaultValue={tab} className="flex-1 min-h-0">
          <TabsList className="h-auto flex-wrap justify-start gap-1 [&>button]:h-8">
            <TabsTrigger value="conductor"><AudioLines />Conductor</TabsTrigger>
            <TabsTrigger value="general">
              <Settings2Icon />
              General
            </TabsTrigger>
            <TabsTrigger value="workflow">
              <WorkflowIcon />
              Workflow
            </TabsTrigger>
            <TabsTrigger value="task-types">
              <TagsIcon />
              Task Types
            </TabsTrigger>
            <TabsTrigger value="checklists">
              <LandPlotIcon />
              Checklists
            </TabsTrigger>
          </TabsList>

          <Separator />

          <TabsContent value="conductor"><ConductorSettingsTab projectId={id} /></TabsContent>

          <TabsContent value="general">
            <ProjectGeneralTab projectId={id} />
          </TabsContent>

          <TabsContent value="workflow">
            <ProjectWorkflowTab projectId={id} />
          </TabsContent>

          <TabsContent value="task-types">
            <ProjectTaskTypesTab projectId={id} />
          </TabsContent>

          <TabsContent value="checklists">
            <EmptyPanel
              title="Checklists"
              description="Group tasks by phase, sprint, or area. The default checklist is picked automatically when a teammate creates a new task without choosing one."
            />
          </TabsContent>
        </Tabs>
      </PageContent>
    </Page>
  );
}
