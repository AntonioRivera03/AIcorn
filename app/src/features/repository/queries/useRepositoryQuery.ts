import { useQuery } from "@tanstack/react-query";
import { apiJson } from "@/lib/api";
import type { RepositoryStatus } from "@/features/repository/repository";

export const repositoryQueryKey = (projectId: number) =>
  ["repository", projectId] as const;

export const repositoryURL = (projectId: number) =>
  `/api/project/${projectId}/settings/repository`;

export function useRepositoryQuery(projectId: number) {
  return useQuery({
    queryKey: repositoryQueryKey(projectId),
    queryFn: ({ signal }) =>
      apiJson<RepositoryStatus>(repositoryURL(projectId), { signal }),
    enabled: projectId > 0,
    // A clone or fetch runs in the background; follow it until it finishes.
    refetchInterval: (query) => (query.state.data?.syncing ? 2000 : false),
  });
}
