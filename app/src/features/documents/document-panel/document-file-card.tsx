import { Download } from "lucide-react";
import { Button } from "@/components/ui/button";
import { documentKind, formatFileSize } from "@/features/documents/document-kind";
import { DocumentKindIcon } from "@/features/documents/document-kind-icon";
import { useDownloadDocumentMutation } from "@/features/documents/queries/use-documents";
import type { ProjectDocument } from "@/features/documents/types";

// For files the browser can't show, like Word documents.
export function DocumentFileCard({ document }: { document: ProjectDocument }) {
  const download = useDownloadDocumentMutation();
  const file = document.file!;
  return (
    <div className="flex flex-wrap items-center gap-3 rounded-lg border p-3">
      <DocumentKindIcon kind={documentKind(document)} className="size-5" />
      <div className="min-w-0 flex-1">
        <p className="break-all text-sm font-medium">{file.name}</p>
        <p className="text-xs text-muted-foreground">
          {formatFileSize(file.size)} · Aycorn can't preview this file type.
        </p>
      </div>
      <Button
        variant="outline"
        size="sm"
        disabled={download.isPending}
        onClick={() => download.mutate(document)}
      >
        <Download />
        Download
      </Button>
    </div>
  );
}
