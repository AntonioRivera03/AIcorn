import { useState } from "react";
import { Drawer, DrawerContent } from "@/components/ui/drawer";
import { DocumentPanelContent } from "@/features/documents/document-panel/document-panel-content";
import type { ProjectDocument } from "@/features/documents/types";
import { useIsMobile } from "@/hooks/useMobile";
import { cn } from "@/lib/utils";

type DocumentPanelProps = {
  // Kept while the panel closes, so its content doesn't vanish mid-animation.
  document: ProjectDocument | undefined;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  tagSuggestions: string[];
};

// Opens from the side like a task. It can widen to the whole window, which
// gives PDFs and images room to zoom.
export function DocumentPanel({
  document,
  open,
  onOpenChange,
  tagSuggestions,
}: DocumentPanelProps) {
  const isMobile = useIsMobile();
  const [expanded, setExpanded] = useState(false);
  const [version, setVersion] = useState(0);

  return (
    <Drawer
      handleOnly={!isMobile}
      repositionInputs={!isMobile}
      direction={isMobile ? "bottom" : "right"}
      open={open && !!document}
      onOpenChange={onOpenChange}
    >
      <DrawerContent
        className={cn(
          "box-border overflow-x-visible rounded-lg p-0 md:min-w-3xl data-[vaul-drawer-direction=bottom]:h-[calc(100dvh-var(--header-height))] data-[vaul-drawer-direction=bottom]:max-h-dvh",
          expanded &&
            "data-[vaul-drawer-direction=right]:w-screen data-[vaul-drawer-direction=right]:sm:max-w-none data-[vaul-drawer-direction=right]:rounded-none",
        )}
      >
        {document && (
          <DocumentPanelContent
            key={`${document.projectId}:${document.id}:${version}`}
            document={document}
            tagSuggestions={tagSuggestions}
            expanded={expanded}
            onToggleExpanded={() => setExpanded((value) => !value)}
            onReload={() => setVersion((value) => value + 1)}
            onDeleted={() => onOpenChange(false)}
          />
        )}
      </DrawerContent>
    </Drawer>
  );
}
