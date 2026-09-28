import { Maximize, Minimize, MoveHorizontal, ZoomIn, ZoomOut } from "lucide-react";
import { ToolbarButton } from "@/features/documents/viewers/viewer-frame/zoom-toolbar/toolbar-button";
import type { Zoom } from "@/features/documents/viewers/use-zoom";

type ZoomToolbarProps = {
  zoom: Zoom;
  info?: string;
  fullscreen: boolean;
  onToggleFullscreen: () => void;
};

export function ZoomToolbar({
  zoom,
  info,
  fullscreen,
  onToggleFullscreen,
}: ZoomToolbarProps) {
  return (
    <div className="flex items-center gap-1 border-b bg-background/80 px-2 py-1">
      {info && <span className="px-1 text-xs text-muted-foreground">{info}</span>}
      <div className="ml-auto flex items-center gap-0.5">
        <ToolbarButton label="Zoom out (-)" onClick={zoom.zoomOut}>
          <ZoomOut />
        </ToolbarButton>
        <span
          className="w-12 text-center text-xs tabular-nums text-muted-foreground"
          aria-live="polite"
        >
          {Math.round(zoom.scale * 100)}%
        </span>
        <ToolbarButton label="Zoom in (+)" onClick={zoom.zoomIn}>
          <ZoomIn />
        </ToolbarButton>
        <ToolbarButton label="Fit width (0)" onClick={zoom.fit} disabled={zoom.fitted}>
          <MoveHorizontal />
        </ToolbarButton>
        <ToolbarButton
          label={fullscreen ? "Exit full screen" : "Full screen"}
          onClick={onToggleFullscreen}
        >
          {fullscreen ? <Minimize /> : <Maximize />}
        </ToolbarButton>
      </div>
    </div>
  );
}
