import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FileText, Plus, Search, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { RichEditor } from "@/features/editor/rich-editor";
import { extractPlainText } from "@/features/task/task-utils";
import { cn } from "@/lib/utils";
import { DocumentDraft, type ProjectDocument } from "./document-draft";

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, init);
  if (!response.ok)
    throw new Error(
      (await response.text()).trim() || "Could not load documents",
    );
  return response.status === 204 ? (undefined as T) : response.json();
}

// Retain unsaved edits across tab/route navigation, including failed requests.
// Successfully saved sessions are discarded when their editor unmounts.
const drafts = new Map<string, DocumentDraft>();

export function DocumentsView({ projectId }: { projectId: number }) {
  const client = useQueryClient();
  const base = `/api/documents/project/${projectId}`;
  const queryKey = ["project-documents", projectId];
  const documents = useQuery({
    queryKey,
    queryFn: () => request<ProjectDocument[]>(base),
  });
  const [selected, setSelected] = useState<number | null>(null);
  const [search, setSearch] = useState("");
  const [editorVersion, setEditorVersion] = useState(0);
  const editor = useRef<DocumentDraft | null>(null);
  const create = useMutation({
    mutationFn: () => request<ProjectDocument>(base, { method: "POST" }),
    onSuccess: (doc) => {
      client.setQueryData<ProjectDocument[]>(queryKey, (items) => [
        doc,
        ...(items ?? []),
      ]);
      setSearch("");
      setSelected(doc.id);
    },
    onError: (error: Error) => toast.error(error.message),
  });
  const items = documents.data ?? [];
  const current = items.find((doc) => doc.id === selected);
  const filtered = items.filter((doc) =>
    `${doc.title}\n${extractPlainText(doc.body)}`
      .toLocaleLowerCase()
      .includes(search.toLocaleLowerCase()),
  );
  const select = async (id: number) => {
    if (editor.current && !(await editor.current.flush())) return;
    setSelected(id);
  };

  return (
    <section
      className="flex min-h-96 flex-1 flex-col overflow-hidden rounded-lg border md:flex-row"
      aria-label="Project documents"
    >
      <aside className="flex max-h-64 flex-col gap-3 border-b p-3 md:max-h-none md:w-64 md:shrink-0 md:border-r md:border-b-0">
        <div className="flex items-center justify-between gap-2">
          <h2 className="font-medium">Documents</h2>
          <Button
            size="sm"
            variant="outline"
            disabled={create.isPending}
            onClick={async () => {
              if (editor.current && !(await editor.current.flush())) return;
              create.mutate();
            }}
          >
            <Plus /> New document
          </Button>
        </div>
        <div className="relative">
          <Search className="pointer-events-none absolute top-2.5 left-2.5 size-4 text-muted-foreground" />
          <Input
            aria-label="Search documents"
            placeholder="Search knowledge…"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            className="pl-8"
          />
        </div>
        {documents.isPending && (
          <p className="text-sm text-muted-foreground">Loading documents…</p>
        )}
        {documents.isError && (
          <div role="alert" className="text-sm text-destructive">
            {documents.error.message}
            <Button variant="link" onClick={() => documents.refetch()}>
              Retry
            </Button>
          </div>
        )}
        <nav
          className="flex min-h-0 flex-col gap-1 overflow-y-auto"
          aria-label="Documents"
        >
          {filtered.map((doc) => (
            <button
              key={doc.id}
              type="button"
              aria-current={current?.id === doc.id ? "page" : undefined}
              className={cn(
                "flex items-center gap-2 rounded-md px-2 py-2 text-left text-sm hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring",
                current?.id === doc.id && "bg-accent",
              )}
              onClick={() => void select(doc.id)}
            >
              <FileText className="size-4 shrink-0 text-muted-foreground" />
              <span className="truncate">
                {doc.title || "Untitled document"}
              </span>
            </button>
          ))}
          {!documents.isPending &&
            !documents.isError &&
            filtered.length === 0 && (
              <p className="p-2 text-sm text-muted-foreground">
                {search
                  ? "No matching documents."
                  : "Keep notes, decisions, and project knowledge here."}
              </p>
            )}
        </nav>
      </aside>
      {current ? (
        <DocumentEditor
          key={`${projectId}:${current.id}:${editorVersion}`}
          document={current}
          onReady={(draft) => {
            editor.current = draft;
          }}
          onReload={() => setEditorVersion((value) => value + 1)}
          onDeleted={() => {
            editor.current = null;
            setSelected(null);
          }}
        />
      ) : (
        <div className="flex flex-1 flex-col items-center justify-center gap-3 p-10 text-center text-muted-foreground">
          <FileText className="size-8" />
          <p>Select a document or create one to start writing.</p>
        </div>
      )}
    </section>
  );
}

function DocumentEditor({
  document,
  onReady,
  onDeleted,
  onReload,
}: {
  document: ProjectDocument;
  onReady: (draft: DocumentDraft | null) => void;
  onDeleted: () => void;
  onReload: () => void;
}) {
  const client = useQueryClient();
  const url = `/api/documents/project/${document.projectId}/${document.id}`;
  const key = `${document.projectId}:${document.id}`;
  const [draft] = useState(() => {
    const existing = drafts.get(key);
    if (existing) return existing;
    const session = new DocumentDraft(document, async (value) => {
      const saved = await request<ProjectDocument>(url, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          title: value.title,
          body: value.body,
          revision: value.revision,
        }),
      });
      client.setQueryData<ProjectDocument[]>(
        ["project-documents", document.projectId],
        (items) => items?.map((item) => (item.id === saved.id ? saved : item)),
      );
      return saved;
    });
    drafts.set(key, session);
    return session;
  });
  const state = useSyncExternalStore(draft.subscribe, draft.getSnapshot);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [reloadOpen, setReloadOpen] = useState(false);
  const discarded = useRef(false);
  const mounted = useRef(false);
  const reload = useMutation({
    mutationFn: () => request<ProjectDocument>(url),
    onSuccess: (saved) => {
      discarded.current = true;
      drafts.delete(key);
      client.setQueryData<ProjectDocument[]>(
        ["project-documents", document.projectId],
        (items) => items?.map((item) => (item.id === saved.id ? saved : item)),
      );
      onReload();
    },
    onError: (error: Error) => toast.error(error.message),
  });
  const remove = useMutation({
    mutationFn: async () => {
      if (!(await draft.flush()))
        throw new Error("Resolve the unsaved document before deleting it.");
      return request<void>(url, {
        method: "DELETE",
        headers: { "If-Match": String(draft.getSnapshot().value.revision) },
      });
    },
    onSuccess: () => {
      drafts.delete(key);
      client.setQueryData<ProjectDocument[]>(
        ["project-documents", document.projectId],
        (items) => items?.filter((item) => item.id !== document.id),
      );
      onDeleted();
    },
    onError: (error: Error) => toast.error(error.message),
  });
  useEffect(() => {
    mounted.current = true;
    drafts.set(key, draft);
    onReady(draft);
    const beforeUnload = (event: BeforeUnloadEvent) => {
      if (draft.getSnapshot().dirty) {
        event.preventDefault();
        event.returnValue = "";
      }
    };
    window.addEventListener("beforeunload", beforeUnload);
    return () => {
      mounted.current = false;
      onReady(null);
      window.removeEventListener("beforeunload", beforeUnload);
      if (!discarded.current)
        void draft.flush().then((saved) => {
          if (saved && !mounted.current && drafts.get(key) === draft)
            drafts.delete(key);
        });
    };
    // Each editor is keyed to one document; callbacks don't own its lifetime.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [draft, key]);

  return (
    <article className="min-w-0 flex-1 overflow-y-auto p-4 sm:p-6">
      <div className="mb-4 flex items-start gap-3">
        <Input
          autoFocus
          aria-label="Document title"
          placeholder="Untitled document"
          value={state.value.title}
          maxLength={500}
          onChange={(event) => draft.edit({ title: event.target.value })}
          onBlur={() => {
            if (!state.error) void draft.flush();
          }}
          className="h-auto border-transparent px-1 text-xl! font-semibold shadow-none"
        />
        <Button
          variant="ghost"
          size="icon"
          aria-label="Delete document"
          onClick={() => setDeleteOpen(true)}
        >
          <Trash2 className="size-4" />
        </Button>
      </div>
      <div
        className="mb-3 flex items-center gap-2 text-xs text-muted-foreground"
        role="status"
      >
        {state.error ? (
          <>
            <span className="text-destructive">
              {state.error} Your edits are retained.
            </span>
            <Button
              size="sm"
              variant="outline"
              onClick={() => void draft.flush()}
            >
              Retry save
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={() => setReloadOpen(true)}
            >
              Reload saved version
            </Button>
          </>
        ) : state.saving ? (
          "Saving…"
        ) : state.dirty ? (
          "Unsaved changes"
        ) : (
          "All changes saved"
        )}
      </div>
      <RichEditor
        initialValue={state.value.body}
        onValueChange={(body) => draft.edit({ body })}
        className="px-1"
      />
      <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete document?</AlertDialogTitle>
            <AlertDialogDescription>
              “{state.value.title || "Untitled document"}” will be permanently
              deleted.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={remove.isPending}
              onClick={() => remove.mutate()}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <AlertDialog open={reloadOpen} onOpenChange={setReloadOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Reload saved document?</AlertDialogTitle>
            <AlertDialogDescription>
              This discards your unsaved edits and loads the latest saved
              version. Copy any writing you want to keep first.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={reload.isPending}
              onClick={() => reload.mutate()}
            >
              Discard edits and reload
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </article>
  );
}
