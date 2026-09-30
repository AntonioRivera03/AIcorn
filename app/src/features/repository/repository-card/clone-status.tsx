import { CircleAlert, FolderGit2, Loader2, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { RelativeTimeWithTooltip } from "@/components/relative-time-with-tooltip";
import type { RepositoryStatus } from "@/features/repository/repository";
import { sentence } from "@/utils/sentence";

// Where Aycorn's clone of an Official repository stands, and a way to fetch
// it now rather than at the next build or agent run.
export function CloneStatus({
  status,
  fetching,
  onFetch,
}: {
  status: RepositoryStatus;
  fetching: boolean;
  onFetch: () => void;
}) {
  const busy = status.syncing || fetching;
  const label = busy
    ? status.cloned
      ? "Fetching…"
      : "Cloning…"
    : status.cloned
      ? "Cloned"
      : "Not cloned yet";
  const Icon = busy ? Loader2 : status.error ? CircleAlert : FolderGit2;

  return (
    <div className="flex flex-col gap-2 rounded-md border border-border p-3">
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <Icon
          aria-hidden
          className={
            status.error && !busy
              ? "size-4 text-destructive"
              : `size-4 text-muted-foreground ${busy ? "animate-spin" : ""}`
          }
        />
        <span role="status" className="text-sm font-medium">
          {label}
        </span>
        {status.fetchedAt && (
          <RelativeTimeWithTooltip
            date={status.fetchedAt}
            label="· fetched"
            className="text-xs"
          />
        )}
        <Button
          variant="outline"
          size="sm"
          className="ml-auto"
          disabled={busy}
          onClick={onFetch}
        >
          <RefreshCw className="size-3.5" />
          Fetch now
        </Button>
      </div>
      {status.error && (
        <p role="alert" className="text-xs break-words text-destructive">
          {sentence(status.error)}
        </p>
      )}
    </div>
  );
}
