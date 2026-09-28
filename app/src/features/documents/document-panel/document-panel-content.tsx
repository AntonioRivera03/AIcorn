import { useState } from "react";
import { Button } from "@/components/ui/button";
import { DeleteDocumentDialog } from "@/features/documents/document-panel/delete-document-dialog";
import { DocumentContent } from "@/features/documents/document-panel/document-content";
import { DocumentPanelHeader } from "@/features/documents/document-panel/document-panel-header";
import { DocumentProperties } from "@/features/documents/document-panel/document-properties";
import {
  fetchDocument,
  useDeleteDocumentMutation,
  useSetDocumentInList,
} from "@/features/documents/queries/use-documents";
import type { ProjectDocument } from "@/features/documents/types";
import { useDocumentDraft } from "@/features/documents/use-document-draft";
import { toast } from "sonner";

type DocumentPanelContentProps = {
  document: ProjectDocument;
  tagSuggestions: string[];
  expanded: boolean;
  onToggleExpanded: () => void;
  // Remount from the saved version, dropping unsaved edits.
  onReload: () => void;
  onDeleted: () => void;
};

export function DocumentPanelContent({
  document,
  tagSuggestions,
  expanded,
  onToggleExpanded,
  onReload,
  onDeleted,
}: DocumentPanelContentProps) {
  const { draft, state, discard } = useDocumentDraft(document);
  const setInList = useSetDocumentInList(document.projectId);
  const remove = useDeleteDocumentMutation(document.projectId);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [reloading, setReloading] = useState(false);
  const value = state.value;

  const reload = async () => {
    setReloading(true);
    try {
      setInList(await fetchDocument(document));
      discard();
      onReload();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Couldn't reload the document.");
    } finally {
      setReloading(false);
    }
  };

  const confirmDelete = async () => {
    // Deleting checks the revision, so pending edits go first.
    if (state.dirty && !(await draft.flush())) {
      toast.error("Save or reload the document before deleting it.");
      return;
    }
    remove.mutate(draft.getSnapshot().value, {
      onSuccess: () => {
        discard();
        setDeleteOpen(false);
        onDeleted();
      },
    });
  };

  return (
    <>
      <DocumentPanelHeader
        state={state}
        expanded={expanded}
        onToggleExpanded={onToggleExpanded}
        onDelete={() => setDeleteOpen(true)}
      />
      <div
        className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto px-3 py-4 sm:px-6 sm:py-6"
        data-vaul-no-drag
      >
        <input
          aria-label="Document title"
          placeholder="Untitled document"
          autoFocus={!value.title}
          value={value.title}
          maxLength={500}
          onChange={(event) => draft.edit({ title: event.target.value })}
          onBlur={() => {
            if (!state.error) void draft.flush();
          }}
          onKeyDown={(event) => {
            if (event.key === "Enter") event.currentTarget.blur();
          }}
          className="w-full bg-transparent text-2xl font-semibold outline-none placeholder:text-muted-foreground"
        />
        {state.error && (
          <div
            role="alert"
            className="flex flex-wrap items-center gap-2 rounded-lg border border-destructive/40 p-3 text-sm"
          >
            <span className="text-destructive">
              {state.error} Your edits are kept.
            </span>
            <Button size="sm" variant="outline" onClick={() => void draft.flush()}>
              Try again
            </Button>
            <Button size="sm" variant="outline" disabled={reloading} onClick={() => void reload()}>
              Load saved version
            </Button>
          </div>
        )}
        <DocumentProperties
          document={value}
          tagSuggestions={tagSuggestions}
          onTagsChange={(tags) => {
            draft.edit({ tags });
            void draft.flush();
          }}
        />
        <DocumentContent document={value} onBodyChange={(body) => draft.edit({ body })} />
      </div>
      <DeleteDocumentDialog
        title={value.title}
        open={deleteOpen}
        pending={remove.isPending}
        onOpenChange={setDeleteOpen}
        onConfirm={() => void confirmDelete()}
      />
    </>
  );
}
