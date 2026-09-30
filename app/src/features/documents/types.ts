import type { Value } from "platejs";

export type DocumentFile = { name: string; mediaType: string; size: number };

export type EmailDetails = {
  from: string;
  to?: string;
  cc?: string;
  date?: string;
};

export type ProjectDocument = {
  id: number;
  projectId: number;
  title: string;
  body: Value;
  tags: string[];
  details: { email?: EmailDetails };
  revision: number;
  createdAt: string;
  updatedAt: string;
  file?: DocumentFile;
};
