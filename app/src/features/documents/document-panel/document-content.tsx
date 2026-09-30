import type { Value } from "platejs";
import { RichEditor } from "@/features/editor/rich-editor";
import { documentKind } from "@/features/documents/document-kind";
import { DocumentFileCard } from "@/features/documents/document-panel/document-file-card";
import type { ProjectDocument } from "@/features/documents/types";
import { ImageViewer } from "@/features/documents/viewers/image-viewer";
import { PdfViewer } from "@/features/documents/viewers/pdf-viewer";
import { extractPlainText } from "@/features/task/task-utils";

type DocumentContentProps = {
  document: ProjectDocument;
  onBodyChange: (body: Value) => void;
};

// A PDF or image shows in place of the body. Notes written on one before this
// layout still show under it.
export function DocumentContent({ document, onBodyChange }: DocumentContentProps) {
  const { viewer } = documentKind(document);
  const editor = (
    <RichEditor
      initialValue={document.body}
      onValueChange={onBodyChange}
      className="px-1"
    />
  );

  if (viewer === "pdf" || viewer === "image") {
    const hasNotes = extractPlainText(document.body).trim() !== "";
    return (
      <div className="flex flex-col gap-4">
        {viewer === "pdf" ? (
          <PdfViewer document={document} />
        ) : (
          <ImageViewer document={document} />
        )}
        {hasNotes && (
          <section aria-label="Notes" className="flex flex-col gap-1">
            <h3 className="text-sm font-medium">Notes</h3>
            {editor}
          </section>
        )}
      </div>
    );
  }
  if (viewer === "file") {
    return (
      <div className="flex flex-col gap-4">
        <DocumentFileCard document={document} />
        <section aria-label="Notes" className="flex flex-col gap-1">
          <h3 className="text-sm font-medium">Notes</h3>
          {editor}
        </section>
      </div>
    );
  }
  return editor;
}
