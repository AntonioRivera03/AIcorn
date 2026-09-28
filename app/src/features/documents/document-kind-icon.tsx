import { FileCode, FileImage, FileText, FileType, Mail, NotebookPen } from "lucide-react";
import type { DocumentKind } from "@/features/documents/document-kind";
import { cn } from "@/lib/utils";

export function DocumentKindIcon({
  kind,
  className,
}: {
  kind: DocumentKind;
  className?: string;
}) {
  const props = { className: cn("size-4 text-muted-foreground", className), "aria-hidden": true };
  switch (kind.label) {
    case "Note":
      return <NotebookPen {...props} />;
    case "Email":
      return <Mail {...props} />;
    case "PDF":
    case "Word":
      return <FileType {...props} />;
    case ".md":
      return <FileCode {...props} />;
    default:
      return kind.viewer === "image" ? <FileImage {...props} /> : <FileText {...props} />;
  }
}
