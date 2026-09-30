import { Badge } from "@/components/ui/badge";
import { documentKind, formatFileSize } from "@/features/documents/document-kind";
import { DocumentTags } from "@/features/documents/document-panel/document-tags";
import type { ProjectDocument } from "@/features/documents/types";

type DocumentPropertiesProps = {
  document: ProjectDocument;
  tagSuggestions: string[];
  onTagsChange: (tags: string[]) => void;
};

const formatSent = (value: string) => {
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? value
    : new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" }).format(date);
};

export function DocumentProperties({
  document,
  tagSuggestions,
  onTagsChange,
}: DocumentPropertiesProps) {
  const kind = documentKind(document);
  const email = document.details.email;
  const file = document.file;

  const rows: [string, React.ReactNode][] = [
    [
      "Type",
      <span className="flex flex-wrap items-center gap-2">
        <Badge variant="outline">{kind.label}</Badge>
        {file && (
          <span className="break-all text-muted-foreground">
            {file.name} · {formatFileSize(file.size)}
          </span>
        )}
      </span>,
    ],
    [
      "Tags",
      <DocumentTags
        tags={document.tags}
        suggestions={tagSuggestions}
        onChange={onTagsChange}
      />,
    ],
  ];
  if (email) {
    rows.push(["From", email.from]);
    if (email.to) rows.push(["To", email.to]);
    if (email.cc) rows.push(["Cc", email.cc]);
    if (email.date) rows.push(["Sent", formatSent(email.date)]);
  }

  return (
    <dl className="grid grid-cols-[5rem_1fr] items-start gap-x-4 gap-y-2.5 text-sm">
      {rows.map(([label, value]) => (
        <div key={label} className="contents">
          <dt className="py-0.5 text-muted-foreground">{label}</dt>
          <dd className="min-w-0 py-0.5 break-words">{value}</dd>
        </div>
      ))}
    </dl>
  );
}
