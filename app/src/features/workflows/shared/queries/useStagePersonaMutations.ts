import { useMutation, useQueryClient } from "@tanstack/react-query";

type BindStagePersonaInput = {
  readonly stageId: number;
  readonly personaId: number;
};

const STAGE_BEARING_QUERY_ROOTS = [
  "allStages",
  "allWorkflows",
  "projectDetails",
  "projectWorkflowSettings",
  "upcomingTasks",
  "allTasksForRelationship",
  "taskRelationships",
] as const;

export function useStagePersonaMutations(workflowId: number) {
  const queryClient = useQueryClient();

  const invalidateStageData = async () => {
    await Promise.all([
      queryClient.invalidateQueries({
        queryKey: ["workflowDetails", workflowId],
      }),
      ...STAGE_BEARING_QUERY_ROOTS.map((root) =>
        queryClient.invalidateQueries({ queryKey: [root] }),
      ),
    ]);
  };

  const bindPersona = useMutation<boolean, Error, BindStagePersonaInput>({
    mutationFn: async ({ stageId, personaId }) => {
      const response = await fetch(`/api/stage/${stageId}/persona`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ personaId }),
      });
      if (!response.ok) {
        throw new Error((await response.text()) || "Failed to bind persona");
      }
      return response.json() as Promise<boolean>;
    },
    onSuccess: invalidateStageData,
  });

  const unbindPersona = useMutation<boolean, Error, number>({
    mutationFn: async (stageId) => {
      const response = await fetch(`/api/stage/${stageId}/persona`, {
        method: "DELETE",
      });
      if (!response.ok) {
        throw new Error((await response.text()) || "Failed to clear persona");
      }
      return response.json() as Promise<boolean>;
    },
    onSuccess: invalidateStageData,
  });

  return { bindPersona, unbindPersona };
}
