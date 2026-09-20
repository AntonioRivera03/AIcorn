import { describe, expect, it, vi } from "vitest";
import { DocumentDraft, type ProjectDocument } from "./document-draft";

const document: ProjectDocument = {
  id: 1,
  projectId: 1,
  title: "",
  body: [{ type: "p", children: [{ text: "" }] }],
  revision: 1,
  createdAt: "",
  updatedAt: "",
};

describe("document autosave", () => {
  it("serializes edits made during a save using the returned revision", async () => {
    let complete!: (value: ProjectDocument) => void;
    const save = vi
      .fn()
      .mockImplementationOnce(
        () =>
          new Promise<ProjectDocument>((resolve) => {
            complete = resolve;
          }),
      )
      .mockImplementation(async (value: ProjectDocument) => ({
        ...value,
        revision: value.revision + 1,
      }));
    const draft = new DocumentDraft(document, save);
    draft.edit({ title: "First" });
    const flushed = draft.flush();
    draft.edit({
      title: "Latest",
      body: [{ type: "p", children: [{ text: "Knowledge" }] }],
    });
    complete({ ...document, title: "First", revision: 2 });
    expect(await flushed).toBe(true);
    expect(save).toHaveBeenCalledTimes(2);
    expect(save.mock.calls[1][0]).toMatchObject({
      title: "Latest",
      revision: 2,
    });
    expect(draft.getSnapshot()).toMatchObject({
      dirty: false,
      saving: false,
      value: { title: "Latest", revision: 3 },
    });
  });

  it("retains unsaved content on failure and retries without overwriting the revision", async () => {
    const save = vi
      .fn()
      .mockRejectedValueOnce(new Error("Changed elsewhere"))
      .mockImplementation(async (value: ProjectDocument) => ({
        ...value,
        revision: 2,
      }));
    const draft = new DocumentDraft(document, save);
    draft.edit({ title: "Keep this" });
    expect(await draft.flush()).toBe(false);
    expect(draft.getSnapshot()).toMatchObject({
      dirty: true,
      error: "Changed elsewhere",
      value: { title: "Keep this", revision: 1 },
    });
    expect(await draft.flush()).toBe(true);
    expect(draft.getSnapshot().dirty).toBe(false);
  });
});
