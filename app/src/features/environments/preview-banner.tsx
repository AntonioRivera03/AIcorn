import { useQuery } from "@tanstack/react-query";
import { environmentRequest } from "./use-environments";

export function PreviewBanner() {
  const query = useQuery({
    queryKey: ["preview-info"],
    queryFn: () =>
      environmentRequest<{
        preview: boolean;
        branch: string;
        revision: string;
        digest: string;
        mainUrl: string;
      }>("/api/preview"),
    staleTime: Infinity,
    retry: false,
  });
  const info = query.data;
  if (!info?.preview) return null;
  return (
    <aside
      aria-label="Application preview"
      className="sticky top-0 z-40 flex flex-wrap items-center justify-between gap-2 border-b border-conductor/30 bg-background px-4 py-2 text-xs text-conductor"
    >
      <span className="break-all">
        <strong>Preview</strong> · {info.branch} · {info.revision.slice(0, 8)}
        <span className="ml-2 text-muted-foreground">
          Isolated data · AI execution disabled
        </span>
      </span>
      {/^https?:\/\//.test(info.mainUrl) && (
        <a
          className="shrink-0 underline underline-offset-2"
          href={info.mainUrl}
          target="_blank"
          rel="noopener noreferrer"
        >
          Open main AICorn
        </a>
      )}
    </aside>
  );
}
