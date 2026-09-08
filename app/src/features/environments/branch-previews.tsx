import { Link } from "@tanstack/react-router";
import { Box } from "lucide-react";
import { Button } from "@/components/ui/button";
import { EnvironmentCard } from "./environment-card";
import { useEnvironmentMutations, useEnvironments } from "./use-environments";

export function BranchPreviews({
  taskId,
  jobId,
  active,
}: {
  taskId: number;
  jobId: number;
  active: boolean;
}) {
  const environments = useEnvironments(0, taskId);
  const { create } = useEnvironmentMutations(0, taskId);
  const items =
    environments.data?.environments.filter((e) => e.jobId === jobId) ?? [];
  return (
    <section
      className="space-y-2 border-t border-border pt-3"
      aria-label={`Run ${jobId} previews`}
    >
      <div className="flex flex-wrap items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          disabled={active || create.isPending}
          onClick={() =>
            create.mutate({ jobId, requestKey: crypto.randomUUID() })
          }
        >
          <Box className="size-4" />
          {create.isPending ? "Creating preview…" : "Preview branch"}
        </Button>
        {environments.data && (
          <Button variant="link" size="sm" asChild>
            <Link
              to="/project/settings/$projectId"
              params={{ projectId: String(environments.data.projectId) }}
              search={{ tab: "environments" }}
            >
              Environment settings
            </Link>
          </Button>
        )}
      </div>
      {active && (
        <p className="text-xs text-muted-foreground">
          The preview captures the agent’s files after the run finishes.
        </p>
      )}
      {environments.error && (
        <p className="text-xs text-destructive">
          Could not load previews. {environments.error.message}
        </p>
      )}
      {items.map((e) => (
        <div
          key={e.id}
          className="rounded-lg border border-border bg-background"
        >
          <EnvironmentCard environment={e} />
        </div>
      ))}
    </section>
  );
}
