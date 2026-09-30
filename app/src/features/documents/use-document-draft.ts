import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { DocumentDraft } from "@/features/documents/document-draft";
import {
  saveDocument,
  useSetDocumentInList,
} from "@/features/documents/queries/use-documents";
import type { ProjectDocument } from "@/features/documents/types";

// Unsaved edits outlive the panel (closing it, switching tabs, a failed
// save). A draft is dropped once its edits are saved and nothing shows it.
const drafts = new Map<string, DocumentDraft>();

export function useDocumentDraft(document: ProjectDocument) {
  const setInList = useSetDocumentInList(document.projectId);
  const key = `${document.projectId}:${document.id}`;
  const [draft] = useState(() => {
    const existing = drafts.get(key);
    if (existing) return existing;
    const session = new DocumentDraft(document, async (value) => {
      const saved = await saveDocument(value);
      setInList(saved);
      return saved;
    });
    drafts.set(key, session);
    return session;
  });
  const state = useSyncExternalStore(draft.subscribe, draft.getSnapshot);
  const discarded = useRef(false);
  const mounted = useRef(false);

  useEffect(() => {
    mounted.current = true;
    drafts.set(key, draft);
    const beforeUnload = (event: BeforeUnloadEvent) => {
      if (draft.getSnapshot().dirty) {
        event.preventDefault();
        event.returnValue = "";
      }
    };
    window.addEventListener("beforeunload", beforeUnload);
    return () => {
      mounted.current = false;
      window.removeEventListener("beforeunload", beforeUnload);
      if (!discarded.current)
        void draft.flush().then((saved) => {
          if (saved && !mounted.current && drafts.get(key) === draft)
            drafts.delete(key);
        });
    };
  }, [draft, key]);

  // Forget this draft's edits, e.g. before loading the saved version or once
  // the document is deleted.
  const discard = () => {
    discarded.current = true;
    drafts.delete(key);
  };

  return { draft, state, discard };
}
