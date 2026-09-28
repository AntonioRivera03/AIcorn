import React, { useState } from "react";
import { Upload } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Empty, EmptyDescription, EmptyTitle } from "@/components/ui/empty";
import { ItemGroup, ItemSeparator } from "@/components/ui/item";
import { filterDocuments, projectTags, usedTags } from "@/features/documents/document-filter";
import { DocumentPanel } from "@/features/documents/document-panel";
import { DocumentRow } from "@/features/documents/documents-view/document-row";
import { DocumentsToolbar } from "@/features/documents/documents-view/documents-toolbar";
import {
  useCreateDocumentMutation,
  useDocumentsQuery,
  useUploadDocumentMutation,
} from "@/features/documents/queries/use-documents";
import { cn } from "@/lib/utils";

const hasFiles = (event: React.DragEvent) => event.dataTransfer.types.includes("Files");

export function DocumentsView({ projectId }: { projectId: number }) {
  const documents = useDocumentsQuery(projectId);
  const create = useCreateDocumentMutation(projectId);
  const upload = useUploadDocumentMutation(projectId);
  const [search, setSearch] = useState("");
  const [selectedTags, setSelectedTags] = useState<string[]>([]);
  // The id stays set while the panel closes, so it animates out with content.
  const [panel, setPanel] = useState<{ id: number; open: boolean } | null>(null);
  const [dropping, setDropping] = useState(false);

  const items = documents.data ?? [];
  const shown = filterDocuments(items, search, selectedTags);
  const current = items.find((doc) => doc.id === panel?.id);
  const busy = create.isPending || upload.isPending;

  const open = (id: number) => setPanel({ id, open: true });

  const uploadFiles = async (files: File[]) => {
    if (busy || files.length === 0) return;
    let last: number | null = null;
    let uploaded = 0;
    // One request per file: the server stores each as its own document.
    for (const file of files) {
      try {
        last = (await upload.mutateAsync(file)).id;
        uploaded++;
      } catch {
        // The mutation already showed why.
      }
    }
    setSearch("");
    setSelectedTags([]);
    if (files.length === 1 && last !== null) open(last);
    else if (uploaded > 1) toast.success(`Uploaded ${uploaded} files.`);
  };

  const createNote = () =>
    create.mutate(undefined, {
      onSuccess: (doc) => {
        setSearch("");
        setSelectedTags([]);
        open(doc.id);
      },
    });

  return (
    <div
      className="relative flex h-full min-h-0 flex-1 flex-col gap-2"
      aria-label="Project documents"
      onDragOver={(event) => {
        if (!hasFiles(event)) return;
        event.preventDefault();
        setDropping(true);
      }}
      onDragLeave={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDropping(false);
      }}
      onDrop={(event) => {
        if (!hasFiles(event)) return;
        event.preventDefault();
        setDropping(false);
        void uploadFiles(Array.from(event.dataTransfer.files));
      }}
      onPasteCapture={(event) => {
        // Pasting a screenshot anywhere on the list uploads it, unless the
        // paste is going into a field.
        const files = Array.from(event.clipboardData.files);
        if (!files.length || (event.target as HTMLElement).closest("input, textarea, [contenteditable=true]")) return;
        event.preventDefault();
        void uploadFiles(files);
      }}
    >
      <DocumentsToolbar
        search={search}
        onSearchChange={setSearch}
        count={shown.length}
        tags={usedTags(items)}
        selectedTags={selectedTags}
        onSelectedTagsChange={setSelectedTags}
        busy={busy}
        onUpload={(files) => void uploadFiles(files)}
        onCreate={createNote}
      />

      <div className="h-full min-h-0 overflow-auto">
        {documents.isPending ? (
          <p className="p-4 text-sm text-muted-foreground">Loading documents…</p>
        ) : documents.isError ? (
          <p role="alert" className="p-4 text-sm text-destructive">
            {documents.error.message}{" "}
            <Button variant="link" onClick={() => void documents.refetch()}>
              Retry
            </Button>
          </p>
        ) : shown.length > 0 ? (
          <ItemGroup className="box-border h-fit rounded-md p-1">
            {shown.map((doc, index) => (
              <React.Fragment key={doc.id}>
                <DocumentRow
                  document={doc}
                  active={panel?.open === true && panel.id === doc.id}
                  onOpen={() => open(doc.id)}
                />
                {index < shown.length - 1 && <ItemSeparator />}
              </React.Fragment>
            ))}
          </ItemGroup>
        ) : (
          <Empty>
            <EmptyTitle>{items.length ? "No matching documents" : "No documents yet"}</EmptyTitle>
            <EmptyDescription>
              {items.length
                ? "Try another search or tag."
                : "Write a note, or drop files here: PDFs, Word files, images, emails (.eml), text, or Markdown, up to 20 MB each. You can also paste a screenshot."}
            </EmptyDescription>
          </Empty>
        )}
      </div>

      <div
        aria-hidden
        className={cn(
          "pointer-events-none absolute inset-0 flex items-center justify-center gap-2 rounded-lg border-2 border-dashed border-primary bg-background/80 text-sm font-medium opacity-0 transition-opacity",
          dropping && "opacity-100",
        )}
      >
        <Upload className="size-4" />
        Drop to upload
      </div>

      <DocumentPanel
        document={current}
        open={panel?.open ?? false}
        onOpenChange={(next) => setPanel((value) => (value ? { ...value, open: next } : value))}
        tagSuggestions={projectTags(items)}
      />
    </div>
  );
}
