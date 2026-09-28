import {
  ChevronsRight,
  Download,
  Ellipsis,
  Maximize2,
  Minimize2,
  Trash2,
} from "lucide-react";
import { RelativeTimeWithTooltip } from "@/components/relative-time-with-tooltip";
import { Button } from "@/components/ui/button";
import {
  DrawerClose,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@/components/ui/drawer";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { documentTime } from "@/features/documents/document-kind";
import type { DocumentState } from "@/features/documents/document-draft";
import { useDownloadDocumentMutation } from "@/features/documents/queries/use-documents";
import { useIsMobile } from "@/hooks/useMobile";

type DocumentPanelHeaderProps = {
  state: DocumentState;
  expanded: boolean;
  onToggleExpanded: () => void;
  onDelete: () => void;
};

export function DocumentPanelHeader({
  state,
  expanded,
  onToggleExpanded,
  onDelete,
}: DocumentPanelHeaderProps) {
  const isMobile = useIsMobile();
  const download = useDownloadDocumentMutation();
  const document = state.value;
  const status = state.error
    ? null
    : state.saving
      ? "Saving…"
      : state.dirty
        ? "Unsaved changes"
        : null;

  return (
    <DrawerHeader className="flex-row items-center gap-1 p-2 sm:border-b">
      <DrawerTitle className="sr-only">{document.title || "Untitled document"}</DrawerTitle>
      <DrawerDescription className="sr-only">
        Document details and contents
      </DrawerDescription>
      <DrawerClose asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          className="hidden text-muted-foreground sm:flex"
          aria-label="Close document"
        >
          <ChevronsRight />
        </Button>
      </DrawerClose>
      {!isMobile && (
        <Button
          variant="ghost"
          size="icon-sm"
          className="text-muted-foreground"
          aria-label={expanded ? "Shrink panel" : "Expand panel"}
          aria-pressed={expanded}
          onClick={onToggleExpanded}
        >
          {expanded ? <Minimize2 className="size-3.5" /> : <Maximize2 className="size-3.5" />}
        </Button>
      )}
      <RelativeTimeWithTooltip
        date={documentTime(document.updatedAt)}
        label="Modified"
        className="ml-1 hidden text-xs sm:flex"
      />
      <span role="status" className="ml-auto text-xs text-muted-foreground">
        {status}
      </span>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="icon-sm"
            className="text-muted-foreground"
            aria-label="Document actions"
          >
            <Ellipsis />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-44">
          {document.file && (
            <>
              <DropdownMenuItem
                disabled={download.isPending}
                onClick={() => download.mutate(document)}
              >
                <Download />
                Download original
              </DropdownMenuItem>
              <DropdownMenuSeparator />
            </>
          )}
          <DropdownMenuItem variant="destructive" onClick={onDelete}>
            <Trash2 />
            Delete document
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </DrawerHeader>
  );
}
