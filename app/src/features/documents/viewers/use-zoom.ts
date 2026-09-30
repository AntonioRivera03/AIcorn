import { useState } from "react";

const STEPS = [0.25, 0.5, 0.75, 1, 1.25, 1.5, 2, 3, 4];
const MIN = STEPS[0];
const MAX = STEPS[STEPS.length - 1];

export type Zoom = {
  // The scale content renders at: 1 is the file's own size.
  scale: number;
  // True while the content fits the viewer's width.
  fitted: boolean;
  zoomIn: () => void;
  zoomOut: () => void;
  fit: () => void;
  // For pinch and ctrl+wheel: scale by a factor, within the limits.
  zoomBy: (factor: number) => void;
};

const clamp = (value: number) => Math.min(MAX, Math.max(MIN, value));

// Zoom starts at "fit width" and follows the viewer's width until the user
// zooms; then it holds the chosen scale.
export function useZoom(fitScale: number): Zoom {
  const [manual, setManual] = useState<number | null>(null);
  const scale = manual ?? fitScale;
  return {
    scale,
    fitted: manual === null,
    zoomIn: () => setManual(STEPS.find((step) => step > scale + 0.01) ?? MAX),
    zoomOut: () =>
      setManual([...STEPS].reverse().find((step) => step < scale - 0.01) ?? MIN),
    fit: () => setManual(null),
    zoomBy: (factor) => setManual(clamp(scale * factor)),
  };
}
