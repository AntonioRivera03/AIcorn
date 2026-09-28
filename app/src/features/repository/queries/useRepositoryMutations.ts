import { useMutation, useQueryClient } from "@tanstack/react-query";
import { apiJson } from "@/lib/api";
import type {
  RepositoryPatch,
  RepositoryStatus,
} from "@/features/repository/repository";
import {
  repositoryQueryKey,
  repositoryURL,
} from "@/features/repository/queries/useRepositoryQuery";

export function useRepositoryMutations(projectId: number) {
  const client = useQueryClient();
  const refresh = (status?: RepositoryStatus) => {
    if (status) client.setQueryData(repositoryQueryKey(projectId), status);
    else
      void client.invalidateQueries({
        queryKey: repositoryQueryKey(projectId),
      });
    // Projects carry the link, and the preview branch list follows it.
    for (const queryKey of [
      ["projectDetails", projectId],
      ["projectWorkflowSettings", projectId],
      ["allProjects"],
      ["pinnedProjects"],
      ["ai-projects"],
      ["environment-branches", projectId],
    ]) {
      void client.invalidateQueries({ queryKey });
    }
  };
  const update = useMutation({
    scope: { id: `repository-${projectId}` },
    mutationFn: (patch: RepositoryPatch) =>
      apiJson<RepositoryStatus>(repositoryURL(projectId), {
        method: "PUT",
        body: JSON.stringify(patch),
      }),
    onSuccess: (status) => refresh(status),
  });
  const fetchNow = useMutation({
    scope: { id: `repository-${projectId}` },
    mutationFn: () =>
      apiJson<RepositoryStatus>(`${repositoryURL(projectId)}/fetch`, {
        method: "POST",
      }),
    onSuccess: (status) => refresh(status),
    // The failure is recorded on the status too; show it there.
    onError: () => refresh(),
  });
  return { update, fetchNow };
}
