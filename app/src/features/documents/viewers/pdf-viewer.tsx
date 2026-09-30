import { useEffect, useRef, useState } from "react";
import type { PDFDocumentProxy } from "pdfjs-dist";
import { Skeleton } from "@/components/ui/skeleton";
import { useDocumentFileQuery } from "@/features/documents/queries/use-documents";
import type { ProjectDocument } from "@/features/documents/types";
import { loadPdfJs } from "@/features/documents/viewers/load-pdfjs";
import { PdfPage } from "@/features/documents/viewers/pdf-viewer/pdf-page";
import { useElementWidth } from "@/features/documents/viewers/use-element-width";
import { useZoom } from "@/features/documents/viewers/use-zoom";
import { ViewerFrame } from "@/features/documents/viewers/viewer-frame";

type PageSize = { width: number; height: number };

// Padding inside the scroll area, so a fitted page doesn't touch the edges.
const GUTTER = 32;

export function PdfViewer({ document }: { document: ProjectDocument }) {
  const file = useDocumentFileQuery(document);
  const [pdf, setPdf] = useState<PDFDocumentProxy | null>(null);
  const [firstPage, setFirstPage] = useState<PageSize | null>(null);
  const [error, setError] = useState<string | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const width = useElementWidth(scrollRef);
  const fitScale =
    firstPage && width > GUTTER ? (width - GUTTER) / firstPage.width : 1;
  const zoom = useZoom(fitScale);

  useEffect(() => {
    const blob = file.data;
    if (!blob) return;
    let cancelled = false;
    let destroy: (() => Promise<void>) | undefined;
    void (async () => {
      try {
        const pdfjs = await loadPdfJs();
        // PDF.js takes ownership of the bytes, so it gets its own copy.
        const task = pdfjs.getDocument({ data: new Uint8Array(await blob.arrayBuffer()) });
        destroy = () => task.destroy();
        const loaded = await task.promise;
        const page = await loaded.getPage(1);
        const viewport = page.getViewport({ scale: 1 });
        if (cancelled) return;
        setFirstPage({ width: viewport.width, height: viewport.height });
        setPdf(loaded);
      } catch (cause) {
        if (!cancelled)
          setError(cause instanceof Error ? cause.message : "This PDF couldn't be opened.");
      }
    })();
    return () => {
      cancelled = true;
      void destroy?.();
    };
  }, [file.data]);

  const failure = file.error?.message ?? error;
  const pages = pdf?.numPages ?? 0;

  return (
    <ViewerFrame
      label={`PDF: ${document.title || document.file?.name}`}
      zoom={zoom}
      scrollRef={scrollRef}
      info={pages ? `${pages} ${pages === 1 ? "page" : "pages"}` : undefined}
      tall
    >
      {failure ? (
        <p role="alert" className="text-sm text-destructive">
          {failure}
        </p>
      ) : pdf && firstPage ? (
        <div className="flex w-max min-w-full flex-col items-center gap-4">
          {Array.from({ length: pages }, (_, index) => (
            <PdfPage
              key={index}
              pdf={pdf}
              pageNumber={index + 1}
              scale={zoom.scale}
              placeholder={firstPage}
              root={scrollRef}
            />
          ))}
        </div>
      ) : (
        <div className="mx-auto flex max-w-xl flex-col gap-3" aria-label="Loading PDF">
          <Skeleton className="aspect-[1/1.3] w-full" />
        </div>
      )}
    </ViewerFrame>
  );
}
