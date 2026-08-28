import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

export function useRequestAgent() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (taskId: number) =>
      fetch(`/api/task/${taskId}/request-agent`, { method: "POST" }).then(
        async (r) => {
          if (!r.ok) throw new Error(await r.text());
          return r.json();
        },
      ),
    onSuccess: () => {
      toast.success("Agent requested — job queued");
      qc.invalidateQueries({ queryKey: ["agentJobs"] });
      qc.invalidateQueries({ queryKey: ["agent-jobs"] });
      qc.invalidateQueries({ queryKey: ["activeAgentJobs"] });
      qc.invalidateQueries({ queryKey: ["active-agent-jobs"] });
    },
    onError: (err: Error) => {
      const raw = err?.message?.trim() ?? "";
      if (raw) {
        toast.error(raw);
      } else {
        toast.error(
          "No persona bound to this stage — bind in Workflow Editor",
        );
      }
    },
  });
}
