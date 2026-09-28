import { useEffect, useRef, useState, type ReactNode, type RefObject } from "react";
import { toast } from "sonner";
import { ZoomToolbar } from "@/features/documents/viewers/viewer-frame/zoom-toolbar";
import type { Zoom } from "@/features/documents/viewers/use-zoom";
import { cn } from "@/lib/utils";

type ViewerFrameProps = {
  label: string;
  zoom: Zoom;
  // Measured by the viewer to fit its content to the width.
  scrollRef: RefObject<HTMLDivElement | null>;
  info?: string;
  // Fixed height for paged content (PDF); images size to themselves.
  tall?: boolean;
  children: ReactNode;
};

// A zoomable, scrollable frame that can go full screen. Keys while it's
// focused: + and - zoom, 0 fits the width. Ctrl/⌘ + wheel (or a trackpad
// pinch) zooms too.
export function ViewerFrame({
  label,
  zoom,
  scrollRef,
  info,
  tall = false,
  children,
}: ViewerFrameProps) {
  const frame = useRef<HTMLElement>(null);
  const [fullscreen, setFullscreen] = useState(false);
  // The wheel listener lives across renders; this keeps its zoom current.
  const zoomBy = useRef(zoom.zoomBy);
  useEffect(() => {
    zoomBy.current = zoom.zoomBy;
  });

  useEffect(() => {
    const onChange = () =>
      setFullscreen(document.fullscreenElement === frame.current);
    document.addEventListener("fullscreenchange", onChange);
    return () => document.removeEventListener("fullscreenchange", onChange);
  }, []);

  useEffect(() => {
    const element = scrollRef.current;
    if (!element) return;
    // Not a React handler: preventing the browser's page zoom needs a
    // non-passive listener.
    const onWheel = (event: WheelEvent) => {
      if (!event.ctrlKey && !event.metaKey) return;
      event.preventDefault();
      zoomBy.current(Math.exp(-event.deltaY / 300));
    };
    element.addEventListener("wheel", onWheel, { passive: false });
    return () => element.removeEventListener("wheel", onWheel);
  }, [scrollRef]);

  const toggleFullscreen = () => {
    if (document.fullscreenElement) {
      void document.exitFullscreen();
      return;
    }
    // Embedded browsers can refuse full screen.
    frame.current
      ?.requestFullscreen()
      .catch(() =>
        toast.error("Full screen isn't available here. Use Expand panel instead."),
      );
  };

  return (
    <section
      ref={frame}
      aria-label={label}
      className={cn(
        "flex flex-col overflow-hidden rounded-lg border bg-muted/40",
        fullscreen && "rounded-none border-0 bg-background",
      )}
    >
      <ZoomToolbar
        zoom={zoom}
        info={info}
        fullscreen={fullscreen}
        onToggleFullscreen={toggleFullscreen}
      />
      <div
        ref={scrollRef}
        tabIndex={0}
        aria-label={`${label}. Press plus or minus to zoom, 0 to fit.`}
        data-vaul-no-drag
        className={cn(
          "overflow-auto p-4 outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset",
          fullscreen ? "flex-1" : tall ? "h-[calc(100dvh-16rem)] min-h-96" : "max-h-[75vh]",
        )}
        // Clicking a page focuses the viewer, so the zoom keys work.
        onPointerDown={(event) => event.currentTarget.focus({ preventScroll: true })}
        onKeyDown={(event) => {
          if (event.metaKey || event.ctrlKey || event.altKey) return;
          const actions: Record<string, () => void> = {
            "+": zoom.zoomIn,
            "=": zoom.zoomIn,
            "-": zoom.zoomOut,
            "0": zoom.fit,
          };
          const action = actions[event.key];
          if (!action) return;
          event.preventDefault();
          event.stopPropagation();
          action();
        }}
      >
        {children}
      </div>
    </section>
  );
}
