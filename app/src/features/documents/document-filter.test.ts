import { describe, expect, it } from "vitest";
import { filterDocuments, projectTags, usedTags } from "@/features/documents/document-filter";
import type { ProjectDocument } from "@/features/documents/types";

const doc = (id: number, title: string, tags: string[], extra: Partial<ProjectDocument> = {}): ProjectDocument => ({
  id,
  projectId: 1,
  title,
  body: [{ type: "p", children: [{ text: `${title} body` }] }],
  tags,
  details: {},
  revision: 1,
  createdAt: "",
  updatedAt: "",
  ...extra,
});

const documents = [
  doc(1, "Spec", ["Draft"]),
  doc(2, "Contract", ["final", "Legal"]),
  doc(3, "Request", [], { details: { email: { from: "Fabiana <f@example.com>" } } }),
];

describe("filterDocuments", () => {
  it("searches titles, text, tags, and email senders", () => {
    expect(filterDocuments(documents, "legal", []).map((d) => d.id)).toEqual([2]);
    expect(filterDocuments(documents, "fabiana", []).map((d) => d.id)).toEqual([3]);
    expect(filterDocuments(documents, "spec body", []).map((d) => d.id)).toEqual([1]);
  });

  it("keeps documents with any selected tag, ignoring case", () => {
    expect(filterDocuments(documents, "", ["Final", "Draft"]).map((d) => d.id)).toEqual([1, 2]);
  });
});

describe("tags", () => {
  it("suggests project tags before the defaults, without repeats", () => {
    expect(projectTags(documents)).toEqual(["Draft", "final", "Legal", "In review"]);
  });

  it("filters only by tags in use", () => {
    expect(usedTags(documents)).toEqual(["Draft", "final", "Legal"]);
  });
});
