import { useEffect, useMemo, useRef, useState } from "react";
import { Skeleton } from "@/components/ui/skeleton";
import { useDocumentFileQuery } from "@/features/documents/queries/use-documents";
import type { ProjectDocument } from "@/features/documents/types";
import { useElementWidth } from "@/features/documents/viewers/use-element-width";
import { useZoom } from "@/features/documents/viewers/use-zoom";
import { ViewerFrame } from "@/features/documents/viewers/viewer-frame";

const GUTTER = 32;

export function ImageViewer({ document }: { document: ProjectDocument }) {
  const file = useDocumentFileQuery(document);
  const [natural, setNatural] = useState<{ width: number; height: number } | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const width = useElementWidth(scrollRef);
  // Small images show at their own size rather than being blown up.
  const fitScale =
    natural && width > GUTTER ? Math.min(1, (width - GUTTER) / natural.width) : 1;
  const zoom = useZoom(fitScale);

  const url = useMemo(
    () => (file.data ? URL.createObjectURL(file.data) : null),
    [file.data],
  );
  useEffect(() => {
    if (!url) return;
    return () => URL.revokeObjectURL(url);
  }, [url]);

  return (
    <ViewerFrame
      label={`Image: ${document.title || document.file?.name}`}
      zoom={zoom}
      scrollRef={scrollRef}
      info={natural ? `${natural.width} × ${natural.height}` : undefined}
    >
      {file.error ? (
        <p role="alert" className="text-sm text-destructive">
          {file.error.message}
        </p>
      ) : url ? (
        <div className="flex w-max min-w-full justify-center">
          <img
            src={url}
            alt={document.title || document.file?.name || "Image"}
            className="block max-w-none"
            style={
              natural
                ? { width: natural.width * zoom.scale, height: natural.height * zoom.scale }
                : { maxWidth: "100%" }
            }
            onLoad={(event) =>
              setNatural({
                width: event.currentTarget.naturalWidth,
                height: event.currentTarget.naturalHeight,
              })
            }
          />
        </div>
      ) : (
        <Skeleton className="aspect-video w-full" />
      )}
    </ViewerFrame>
  );
}
