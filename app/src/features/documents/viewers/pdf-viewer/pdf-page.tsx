import { useEffect, useRef, useState, type RefObject } from "react";
import type { PDFDocumentProxy, RenderTask } from "pdfjs-dist";

type PdfPageProps = {
  pdf: PDFDocumentProxy;
  pageNumber: number;
  scale: number;
  // The first page's size, used until this page's own size is known.
  placeholder: { width: number; height: number };
  // The scrolling viewer, so only pages near the view get drawn.
  root: RefObject<HTMLElement | null>;
};

export function PdfPage({ pdf, pageNumber, scale, placeholder, root }: PdfPageProps) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const [near, setNear] = useState(pageNumber <= 2);
  const [size, setSize] = useState(placeholder);

  useEffect(() => {
    const element = canvas.current;
    if (!element || near) return;
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) setNear(true);
      },
      { root: root.current, rootMargin: "800px 0px" },
    );
    observer.observe(element);
    return () => observer.disconnect();
  }, [near, root]);

  useEffect(() => {
    if (!near) return;
    let cancelled = false;
    let task: RenderTask | undefined;
    void pdf.getPage(pageNumber).then((page) => {
      const element = canvas.current;
      if (cancelled || !element) return;
      const base = page.getViewport({ scale: 1 });
      setSize({ width: base.width, height: base.height });
      // Draw at the screen's pixel density so text stays sharp.
      const ratio = window.devicePixelRatio || 1;
      const viewport = page.getViewport({ scale: scale * ratio });
      element.width = Math.floor(viewport.width);
      element.height = Math.floor(viewport.height);
      task = page.render({ canvas: element, viewport });
      task.promise.catch(() => {
        // Cancelled by a newer zoom level, or the viewer closed.
      });
    });
    return () => {
      cancelled = true;
      task?.cancel();
    };
  }, [pdf, pageNumber, scale, near]);

  return (
    <canvas
      ref={canvas}
      role="img"
      aria-label={`Page ${pageNumber}`}
      className="block shrink-0 bg-white shadow-sm"
      style={{ width: size.width * scale, height: size.height * scale }}
    />
  );
}
