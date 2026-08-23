import { useMemo } from "react";
import { toast } from "sonner";
import { BulkActionsToolbarBase } from "@/components/bulk-actions-toolbar-base";
import { bulkResultToast } from "@/features/workflows/shared/bulk-result-toast";
import { useSharedSelection } from "@/hooks/useSelection";
import type { Persona } from "@/types/types";
import { usePersonaMutations } from "@/features/persona/queries/use-persona-mutations";

type PersonasBulkActionsToolbarProps = {
  personas: Persona[];
  stageCounts: ReadonlyMap<number, number>;
};

export function PersonasBulkActionsToolbar({
  personas,
  stageCounts,
}: PersonasBulkActionsToolbarProps) {
  const { selectedIds, clearSelection } = useSharedSelection();
  const { bulkDeletePersonas } = usePersonaMutations();
  const selectedPersonas = useMemo(
    () => personas.filter((persona) => selectedIds.has(`persona-${persona.ID}`)),
    [personas, selectedIds],
  );
  const ids = selectedPersonas.map((persona) => persona.ID);
  const boundStages = selectedPersonas.reduce(
    (total, persona) => total + (stageCounts.get(persona.ID) ?? 0),
    0,
  );

  const handleDelete = () => {
    bulkDeletePersonas.mutate(ids, {
      onSuccess: (result) => {
        bulkResultToast(
          result,
          `Deleted ${result.success} persona${result.success === 1 ? "" : "s"}.`,
        );
        clearSelection();
      },
      onError: () => toast.error("Failed to delete personas."),
    });
  };

  const count = selectedPersonas.length;

  return (
    <BulkActionsToolbarBase
      count={count}
      onClear={clearSelection}
      delete={{
        onConfirm: handleDelete,
        title: `Delete ${count} persona${count === 1 ? "" : "s"}?`,
        description:
          boundStages > 0
            ? `This will unbind ${boundStages} stage${boundStages === 1 ? "" : "s"}. This action cannot be undone.`
            : "Any stages using these personas will be unbound. This action cannot be undone.",
        busy: bulkDeletePersonas.isPending,
      }}
    />
  );
}
