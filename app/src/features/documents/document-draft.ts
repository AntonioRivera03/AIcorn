import type { Value } from "platejs";

export type ProjectDocument = {
  id: number;
  projectId: number;
  title: string;
  body: Value;
  revision: number;
  createdAt: string;
  updatedAt: string;
  file?: { name: string; mediaType: string; size: number };
};

export type DocumentState = {
  value: ProjectDocument;
  saving: boolean;
  dirty: boolean;
  error: string | null;
};

// A serial autosave queue keeps edits made during a request and advances the
// revision only after that request succeeds. The view can flush before leaving.
export class DocumentDraft {
  private saved: ProjectDocument;
  private state: DocumentState;
  private listeners = new Set<() => void>();
  private timer: ReturnType<typeof setTimeout> | undefined;
  private pending: Promise<boolean> | undefined;
  private save: (value: ProjectDocument) => Promise<ProjectDocument>;

  constructor(
    value: ProjectDocument,
    save: (value: ProjectDocument) => Promise<ProjectDocument>,
  ) {
    this.save = save;
    this.saved = value;
    this.state = { value, saving: false, dirty: false, error: null };
  }
  getSnapshot = () => this.state;
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  private emit(patch: Partial<DocumentState>) {
    this.state = { ...this.state, ...patch };
    this.listeners.forEach((listener) => listener());
  }
  edit(patch: Partial<Pick<ProjectDocument, "title" | "body">>) {
    const value = { ...this.state.value, ...patch };
    const dirty =
      value.title !== this.saved.title ||
      JSON.stringify(value.body) !== JSON.stringify(this.saved.body);
    this.emit({ value, dirty });
    clearTimeout(this.timer);
    // A failed write needs an explicit retry so a conflict isn't overwritten.
    if (dirty && !this.state.error)
      this.timer = setTimeout(() => void this.flush(), 400);
  }
  flush = (): Promise<boolean> => {
    clearTimeout(this.timer);
    if (this.pending) return this.pending;
    if (!this.state.dirty) return Promise.resolve(true);
    this.emit({ saving: true, error: null });
    this.pending = this.drain().finally(() => {
      this.pending = undefined;
    });
    return this.pending;
  };
  private async drain() {
    try {
      while (this.state.dirty) {
        const sent = { ...this.state.value, revision: this.saved.revision };
        const saved = await this.save(sent);
        this.saved = saved;
        const value = {
          ...saved,
          title: this.state.value.title,
          body: this.state.value.body,
        };
        const dirty =
          value.title !== saved.title ||
          JSON.stringify(value.body) !== JSON.stringify(saved.body);
        this.emit({ value, dirty });
      }
      this.emit({ saving: false });
      return true;
    } catch (error) {
      this.emit({
        saving: false,
        error:
          error instanceof Error
            ? error.message
            : "Document could not be saved",
      });
      return false;
    }
  }
}
