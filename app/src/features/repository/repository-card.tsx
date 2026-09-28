import { useState } from "react";
import { toast } from "sonner";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { CloneStatus } from "@/features/repository/repository-card/clone-status";
import { LinkField } from "@/features/repository/repository-card/link-field";
import { RemoveCloneDialog } from "@/features/repository/repository-card/remove-clone-dialog";
import { useRepositoryMutations } from "@/features/repository/queries/useRepositoryMutations";
import { useRepositoryQuery } from "@/features/repository/queries/useRepositoryQuery";
import {
  githubURLProblem,
  removesClone,
  repoPathProblem,
  repositoryName,
  type RepositoryPatch,
} from "@/features/repository/repository";
import type { RepoMode } from "@/types/types";

const MODE_HINTS: Record<RepoMode, string> = {
  "": "Choose how agents and branch environments reach this project's code.",
  personal:
    "Aycorn works from a folder on the server that you keep up to date yourself.",
  official:
    "Aycorn clones the GitHub repository itself and fetches it before every environment build and agent run. It never pushes.",
};

// How the project reaches its code: a Personal local folder, or an Official
// GitHub repository Aycorn manages. Edits save as you leave a field.
export function RepositoryCard({ projectId }: { projectId: number }) {
  const repository = useRepositoryQuery(projectId);
  const { update, fetchNow } = useRepositoryMutations(projectId);
  // A change that would delete Aycorn's clone, waiting for confirmation.
  const [pending, setPending] = useState<RepositoryPatch | null>(null);
  // Bumped to put the saved values back in the fields.
  const [fieldGeneration, setFieldGeneration] = useState(0);
  const status = repository.data;

  const apply = (patch: RepositoryPatch) =>
    update.mutate(patch, {
      onError: (error) =>
        toast.error(error.message || "Couldn't update the repository link."),
    });
  const save = (patch: RepositoryPatch) => {
    if (status && removesClone(status, patch)) setPending(patch);
    else apply(patch);
  };
  const cancelPending = () => {
    setPending(null);
    setFieldGeneration((n) => n + 1);
  };

  return (
    <Card className="w-full max-w-md gap-2 rounded-lg py-4 shadow-none">
      <CardHeader className="px-4 gap-1">
        <CardTitle className="font-medium">Repository</CardTitle>
        <CardDescription className="text-xs text-muted-foreground">
          Where agents and branch environments get this project's code. Changes
          save automatically.
        </CardDescription>
      </CardHeader>
      <CardContent className="px-4 flex flex-col gap-3">
        {!status ? (
          <p className="text-sm text-muted-foreground">
            {repository.isError
              ? `Couldn't load the repository link. ${repository.error.message}`
              : "Loading…"}
          </p>
        ) : (
          <>
            <div className="flex flex-col gap-1.5">
              <ToggleGroup
                type="single"
                variant="outline"
                value={status.mode}
                aria-label="Repository mode"
                onValueChange={(mode) => {
                  if (
                    (mode === "personal" || mode === "official") &&
                    mode !== status.mode
                  ) {
                    save({ mode });
                  }
                }}
              >
                <ToggleGroupItem value="personal">Personal</ToggleGroupItem>
                <ToggleGroupItem value="official">Official</ToggleGroupItem>
              </ToggleGroup>
              <p className="text-xs text-muted-foreground">
                {MODE_HINTS[status.mode]}
              </p>
            </div>
            {status.mode === "personal" && (
              <LinkField
                key={`path-${fieldGeneration}`}
                id={`repository-path-${projectId}`}
                label="Local folder"
                value={status.path}
                placeholder="/home/you/projects/app"
                hint="An absolute path to a git checkout on the server."
                problem={repoPathProblem}
                onSave={(path) => save({ path })}
              />
            )}
            {status.mode === "official" && (
              <>
                <LinkField
                  key={`url-${fieldGeneration}`}
                  id={`repository-url-${projectId}`}
                  label="GitHub repository"
                  value={status.url}
                  placeholder="https://github.com/owner/repo"
                  hint="Cloned with the server's git login (gh auth login, or a credential helper)."
                  problem={githubURLProblem}
                  onSave={(url) => save({ url })}
                />
                {status.url && (
                  <CloneStatus
                    status={status}
                    fetching={fetchNow.isPending}
                    onFetch={() =>
                      fetchNow.mutate(undefined, {
                        onError: (error) => toast.error(error.message),
                      })
                    }
                  />
                )}
              </>
            )}
            <RemoveCloneDialog
              open={pending !== null}
              repository={repositoryName(status.url)}
              switching={pending?.mode !== undefined}
              onCancel={cancelPending}
              onConfirm={() => {
                if (pending) apply(pending);
                setPending(null);
              }}
            />
          </>
        )}
      </CardContent>
    </Card>
  );
}
