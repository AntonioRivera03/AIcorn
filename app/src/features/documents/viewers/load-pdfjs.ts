import workerUrl from "pdfjs-dist/build/pdf.worker.min.mjs?url";

let loading: Promise<typeof import("pdfjs-dist")> | null = null;

// PDF.js is large, so it loads the first time a PDF is opened.
export const loadPdfJs = () =>
  (loading ??= import("pdfjs-dist").then((pdfjs) => {
    pdfjs.GlobalWorkerOptions.workerSrc = workerUrl;
    return pdfjs;
  }));
