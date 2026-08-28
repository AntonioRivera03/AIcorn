import { useEffect, useState } from "react";
import { toast } from "sonner";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useProjectWorkflowSettingsQuery } from "@/features/settings/project-workflow/queries/useProjectWorkflowSettingsQuery";
import { useProjectMutation } from "@/queries/useProjectMutation";
import { PROJECT_VIEWS } from "@/types/types";

const VIEW_LABELS: Record<string, string> = {
  list: "List",
  kanban: "Kanban",
  month: "Month",
  week: "Week",
};

export function ProjectGeneralTab({ projectId }: { projectId: number }) {
  const { data, isFetching } = useProjectWorkflowSettingsQuery(projectId);
  const { updateProject } = useProjectMutation(projectId);

  if (!data?.Project) {
    return (
      <div className="flex items-center justify-center rounded-lg border border-dashed py-12 text-sm text-muted-foreground">
        {isFetching ? "Loading project..." : "Project not found."}
      </div>
    );
  }

  const project = data.Project;

  const [repoPathDraft, setRepoPathDraft] = useState(project.RepoPath ?? "");

  useEffect(() => {
    setRepoPathDraft(project.RepoPath ?? "");
  }, [project.RepoPath]);

  const repoPathTrimmed = repoPathDraft.trim();
  const repoPathValidation =
    repoPathTrimmed === ""
      ? null
      : repoPathTrimmed.includes("~")
        ? "Avoid ~ — use an absolute path."
        : !repoPathTrimmed.startsWith("/")
          ? "Use an absolute path starting with /."
          : null;

  const handleViewChange = (view: string) => {
    updateProject.mutate(
      { ...project, DefaultView: view },
      {
        onError: () => toast.error("Failed to update default view."),
      },
    );
  };

  const handleRepoPathBlur = () => {
    const trimmed = repoPathDraft.trim();
    if (trimmed === (project.RepoPath ?? "")) return;
    updateProject.mutate(
      { ...project, RepoPath: trimmed },
      {
        onError: () => toast.error("Failed to update repo folder."),
      },
    );
  };

  return (
    <section className="flex flex-col gap-6">
      <div className="flex flex-col gap-1">
        <h2 className="text-lg font-medium">General</h2>
        <p className="max-w-prose text-sm text-muted-foreground">
          Project-wide preferences.
        </p>
      </div>

      <Card className="w-full max-w-md gap-2 rounded-lg py-4 shadow-none">
        <CardHeader className="px-4 gap-1">
          <CardTitle className="font-medium">Default view</CardTitle>
          <CardDescription className="text-xs text-muted-foreground">
            The view this project opens on. Changes save automatically.
          </CardDescription>
        </CardHeader>
        <CardContent className="px-4">
          <Select
            value={project.DefaultView || "list"}
            onValueChange={handleViewChange}
          >
            <SelectTrigger className="w-48" aria-label="Default view">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {PROJECT_VIEWS.map((view) => (
                <SelectItem key={view} value={view}>
                  {VIEW_LABELS[view]}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </CardContent>
      </Card>

      <Card className="w-full max-w-md gap-2 rounded-lg py-4 shadow-none">
        <CardHeader className="px-4 gap-1">
          <CardTitle className="font-medium">Linked repo folder</CardTitle>
          <CardDescription className="text-xs text-muted-foreground">
            Absolute path to the git repo this project works in. The coding
            harness runs in a worktree under this folder. Changes save
            automatically.
          </CardDescription>
        </CardHeader>
        <CardContent className="px-4 flex flex-col gap-1.5">
          <Input
            value={repoPathDraft}
            onChange={(e) => setRepoPathDraft(e.target.value)}
            onBlur={handleRepoPathBlur}
            placeholder="/home/hal/Projects/my-app"
            aria-label="Linked repo folder"
          />
          <p className="text-xs text-muted-foreground">
            e.g. /home/hal/Projects/my-app
          </p>
          {repoPathValidation && (
            <p className="text-xs text-muted-foreground">
              {repoPathValidation}
            </p>
          )}
        </CardContent>
      </Card>
    </section>
  );
}
