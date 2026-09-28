import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiFetch, apiJson } from "@/lib/api";
import { sentence } from "@/utils/sentence";
import type { ProjectDocument } from "@/features/documents/types";

export const MAX_UPLOAD_BYTES = 20 * 1024 * 1024;
export const UPLOAD_ACCEPT =
  ".pdf,.doc,.docx,.png,.jpg,.jpeg,.gif,.webp,.txt,.md,.eml";

const documentsUrl = (projectId: number) => `/api/documents/project/${projectId}`;
const documentUrl = (document: Pick<ProjectDocument, "projectId" | "id">) =>
  `${documentsUrl(document.projectId)}/${document.id}`;

export const documentsQueryKey = (projectId: number) => ["project-documents", projectId];

export function useDocumentsQuery(projectId: number) {
  return useQuery({
    queryKey: documentsQueryKey(projectId),
    queryFn: () => apiJson<ProjectDocument[]>(documentsUrl(projectId)),
  });
}

// Puts a saved document into the list without refetching every body.
export function useSetDocumentInList(projectId: number) {
  const client = useQueryClient();
  return (saved: ProjectDocument) =>
    client.setQueryData<ProjectDocument[]>(documentsQueryKey(projectId), (items) =>
      items?.map((item) => (item.id === saved.id ? saved : item)),
    );
}

const usePrependDocument = (projectId: number) => {
  const client = useQueryClient();
  return (document: ProjectDocument) =>
    client.setQueryData<ProjectDocument[]>(documentsQueryKey(projectId), (items) => [
      document,
      ...(items ?? []),
    ]);
};

export function useCreateDocumentMutation(projectId: number) {
  const prepend = usePrependDocument(projectId);
  return useMutation({
    mutationFn: () =>
      apiJson<ProjectDocument>(documentsUrl(projectId), { method: "POST" }),
    onSuccess: prepend,
    onError: (error: Error) => toast.error(error.message),
  });
}

export function useUploadDocumentMutation(projectId: number) {
  const prepend = usePrependDocument(projectId);
  return useMutation({
    mutationFn: async (file: File) => {
      if (file.size > MAX_UPLOAD_BYTES)
        throw new Error("Files must be 20 MB or smaller.");
      const body = new FormData();
      body.append("file", file);
      // Not apiJson: the browser has to set the multipart Content-Type.
      const response = await apiFetch(`${documentsUrl(projectId)}/upload`, {
        method: "POST",
        body,
      });
      if (!response.ok)
        throw new Error(sentence((await response.text()).trim()) || "Upload failed.");
      return (await response.json()) as ProjectDocument;
    },
    onSuccess: prepend,
    onError: (error: Error) => toast.error(error.message),
  });
}

// Title, body, and tags, checked against the revision the edit started from.
export const saveDocument = (value: ProjectDocument) =>
  apiJson<ProjectDocument>(documentUrl(value), {
    method: "PUT",
    body: JSON.stringify({
      title: value.title,
      body: value.body,
      tags: value.tags,
      revision: value.revision,
    }),
  });

export const fetchDocument = (document: Pick<ProjectDocument, "projectId" | "id">) =>
  apiJson<ProjectDocument>(documentUrl(document));

export function useDeleteDocumentMutation(projectId: number) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (document: ProjectDocument) =>
      apiJson<void>(documentUrl(document), {
        method: "DELETE",
        headers: { "If-Match": String(document.revision) },
      }),
    onSuccess: (_, document) =>
      client.setQueryData<ProjectDocument[]>(documentsQueryKey(projectId), (items) =>
        items?.filter((item) => item.id !== document.id),
      ),
    onError: (error: Error) => toast.error(error.message),
  });
}

const fetchFile = async (document: ProjectDocument, download = false) => {
  // Through apiFetch, not a plain <img>/<iframe>/<a> URL: the server needs
  // the workspace header, which only apiFetch sends.
  const response = await apiFetch(
    `${documentUrl(document)}/file${download ? "?download=1" : ""}`,
  );
  if (!response.ok)
    throw new Error(sentence((await response.text()).trim()) || "Couldn't load the file.");
  return response.blob();
};

// The original file never changes, so it's fetched once per document.
export function useDocumentFileQuery(document: ProjectDocument, enabled = true) {
  return useQuery({
    queryKey: ["project-document-file", document.projectId, document.id],
    queryFn: () => fetchFile(document),
    enabled: enabled && !!document.file,
    staleTime: Infinity,
    gcTime: 5 * 60 * 1000,
  });
}

export function useDownloadDocumentMutation() {
  return useMutation({
    mutationFn: async (document: ProjectDocument) => {
      const blob = await fetchFile(document, true);
      const url = URL.createObjectURL(blob);
      const link = Object.assign(window.document.createElement("a"), {
        href: url,
        download: document.file?.name ?? document.title,
      });
      link.click();
      setTimeout(() => URL.revokeObjectURL(url), 0);
    },
    onError: (error: Error) => toast.error(error.message),
  });
}
