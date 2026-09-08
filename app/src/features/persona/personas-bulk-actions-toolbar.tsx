import { useMemo } from "react";
import { toast } from "sonner";
import { BulkActionsToolbarBase } from "@/components/bulk-actions-toolbar-base";
import { bulkResultToast } from "@/features/workflows/shared/bulk-result-toast";
import { useSharedSelection } from "@/hooks/useSelection";
import type { Persona } from "@/types/types";
import { usePersonaMutations } from "@/features/persona/queries/use-persona-mutations";

type PersonasBulkActionsToolbarProps = {
  personas: Persona[];
};

export function PersonasBulkActionsToolbar({
  personas,
}: PersonasBulkActionsToolbarProps) {
  const { selectedIds, clearSelection } = useSharedSelection();
  const { bulkDeletePersonas } = usePersonaMutations();
  const selectedPersonas = useMemo(
    () => personas.filter((persona) => selectedIds.has(`persona-${persona.ID}`)),
    [personas, selectedIds],
  );
  const ids = selectedPersonas.map((persona) => persona.ID);


  const handleDelete = () => {
    bulkDeletePersonas.mutate(ids, {
      onSuccess: (result) => {
        bulkResultToast(
          result,
          `Deleted ${result.success} agent${result.success === 1 ? "" : "s"}.`,
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
        title: `Delete ${count} agent${count === 1 ? "" : "s"}?`,
        description: "The saved instructions will be deleted. Previous AI runs will remain available.",
        busy: bulkDeletePersonas.isPending,
      }}
    />
  );
}
