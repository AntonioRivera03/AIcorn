import type { ProjectDocument } from "@/features/documents/types";

// How a document is shown: a PDF or image gets a zoomable viewer in place of
// the body, other files a download card above their notes, and notes, text,
// Markdown, and email an editable body.
export type DocumentViewer = "pdf" | "image" | "file" | "text";

export type DocumentKind = {
  label: string;
  viewer: DocumentViewer;
};

const extension = (name: string) => {
  const dot = name.lastIndexOf(".");
  return dot > 0 ? name.slice(dot + 1).toLowerCase() : "";
};

export const documentKind = (document: Pick<ProjectDocument, "file">): DocumentKind => {
  const file = document.file;
  if (!file) return { label: "Note", viewer: "text" };
  const ext = extension(file.name);
  if (file.mediaType === "application/pdf") return { label: "PDF", viewer: "pdf" };
  if (file.mediaType.startsWith("image/"))
    return { label: ext ? ext.toUpperCase() : "Image", viewer: "image" };
  if (file.mediaType === "message/rfc822") return { label: "Email", viewer: "text" };
  if (ext === "md") return { label: ".md", viewer: "text" };
  if (ext === "txt") return { label: ".txt", viewer: "text" };
  if (ext === "doc" || ext === "docx") return { label: "Word", viewer: "file" };
  return { label: ext ? `.${ext}` : "File", viewer: "file" };
};

export const formatFileSize = (bytes: number) =>
  bytes < 1024 * 1024
    ? `${Math.max(1, Math.ceil(bytes / 1024))} KB`
    : `${(bytes / (1024 * 1024)).toFixed(1)} MB`;

// The server stores SQLite's UTC "YYYY-MM-DD HH:MM:SS"; without a zone,
// browsers would read it as local time.
export const documentTime = (value: string) =>
  value.includes("T") ? value : `${value.replace(" ", "T")}Z`;
