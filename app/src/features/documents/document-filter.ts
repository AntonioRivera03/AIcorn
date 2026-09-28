import type { ProjectDocument } from "@/features/documents/types";
import { extractPlainText } from "@/features/task/task-utils";

// Offered on documents with no tags yet, so a status is one click away.
export const DEFAULT_TAGS = ["Draft", "In review", "Final"];

// Every tag used in the project, then the defaults, without repeats.
export const projectTags = (documents: ProjectDocument[]) => {
  const seen = new Map<string, string>();
  for (const tag of [...documents.flatMap((doc) => doc.tags), ...DEFAULT_TAGS]) {
    if (!seen.has(tag.toLowerCase())) seen.set(tag.toLowerCase(), tag);
  }
  return [...seen.values()];
};

// The tags that are actually on documents, for filtering.
export const usedTags = (documents: ProjectDocument[]) =>
  projectTags(documents).filter((tag) =>
    documents.some((doc) => doc.tags.some((own) => own.toLowerCase() === tag.toLowerCase())),
  );

// Matches the search against the title, text, tags, and email sender; with
// tags selected, a document needs at least one of them.
export const filterDocuments = (
  documents: ProjectDocument[],
  search: string,
  tags: string[],
) => {
  const query = search.trim().toLocaleLowerCase();
  const wanted = new Set(tags.map((tag) => tag.toLowerCase()));
  return documents.filter((doc) => {
    if (wanted.size && !doc.tags.some((tag) => wanted.has(tag.toLowerCase()))) return false;
    if (!query) return true;
    const haystack = [
      doc.title,
      extractPlainText(doc.body),
      doc.tags.join(" "),
      doc.details.email?.from ?? "",
      doc.file?.name ?? "",
    ].join("\n");
    return haystack.toLocaleLowerCase().includes(query);
  });
};
