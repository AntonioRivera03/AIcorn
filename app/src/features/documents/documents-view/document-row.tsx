import { formatDistanceToNow } from "date-fns";
import { ChevronRight } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import {
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemMedia,
  ItemTitle,
} from "@/components/ui/item";
import { documentKind, documentTime } from "@/features/documents/document-kind";
import { DocumentKindIcon } from "@/features/documents/document-kind-icon";
import type { ProjectDocument } from "@/features/documents/types";
import { cn } from "@/lib/utils";

type DocumentRowProps = {
  document: ProjectDocument;
  active: boolean;
  onOpen: () => void;
};

// Laid out like a task row: the title leads, tags sit where a task's
// assignee does, and the document type takes the stage's place.
export function DocumentRow({ document, active, onOpen }: DocumentRowProps) {
  const kind = documentKind(document);
  const email = document.details.email;
  const updated = formatDistanceToNow(new Date(documentTime(document.updatedAt)), {
    addSuffix: true,
  });

  return (
    <Item asChild className={cn("w-full px-2 text-left hover:bg-secondary sm:px-4", active && "bg-secondary")}>
      <button type="button" onClick={onOpen} aria-current={active || undefined}>
        <ItemMedia>
          <DocumentKindIcon kind={kind} className="size-5" />
        </ItemMedia>
        <ItemContent className="min-w-0">
          <ItemTitle
            className={cn(
              "w-full min-w-0 break-words",
              !document.title && "text-muted-foreground",
            )}
          >
            {document.title || "Untitled document"}
          </ItemTitle>
          <ItemDescription className="line-clamp-none">
            <span className="flex flex-wrap items-center gap-1.5">
              <Badge variant="outline" className="bg-background sm:hidden">
                {kind.label}
              </Badge>
              {document.tags.map((tag) => (
                <Badge key={tag} variant="secondary">
                  {tag}
                </Badge>
              ))}
              {email?.from && (
                <span className="min-w-0 break-all text-xs">From {email.from}</span>
              )}
              <span className="text-xs">Updated {updated}</span>
            </span>
          </ItemDescription>
        </ItemContent>
        <ItemActions>
          <Badge variant="outline" className="hidden bg-background sm:inline-flex">
            {kind.label}
          </Badge>
          <ChevronRight className="size-4" />
        </ItemActions>
      </button>
    </Item>
  );
}
