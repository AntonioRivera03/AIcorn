import { describe, expect, it } from "vitest";
import { decideComposerKey } from "./composer-keys";

const closed = { open: false, optionsCount: 0, selected: 0 };
const noShift = { shift: false };

describe("composer key handling", () => {
  it("sends on plain Enter when the picker is closed, but not with Shift", () => {
    expect(decideComposerKey("Enter", noShift, closed)).toEqual({ type: "submit" });
    expect(decideComposerKey("Enter", { shift: true }, closed)).toEqual({ type: "none" });
    expect(decideComposerKey("a", noShift, closed)).toEqual({ type: "none" });
  });

  it("moves the highlighted option with the arrow keys, wrapping around", () => {
    const open = { open: true, optionsCount: 3, selected: 1 };
    expect(decideComposerKey("ArrowDown", noShift, open)).toEqual({ type: "move", index: 2 });
    expect(decideComposerKey("ArrowUp", noShift, open)).toEqual({ type: "move", index: 0 });
    expect(decideComposerKey("ArrowDown", noShift, { ...open, selected: 2 })).toEqual({
      type: "move",
      index: 0,
    });
  });

  it("lets arrow keys move the caret instead when there are no results yet", () => {
    const empty = { open: true, optionsCount: 0, selected: 0 };
    expect(decideComposerKey("ArrowDown", noShift, empty)).toEqual({ type: "none" });
    expect(decideComposerKey("ArrowUp", noShift, empty)).toEqual({ type: "none" });
  });

  it("inserts the highlighted option on Enter or Tab, and never sends", () => {
    const open = { open: true, optionsCount: 2, selected: 0 };
    expect(decideComposerKey("Enter", noShift, open)).toEqual({ type: "choose" });
    expect(decideComposerKey("Tab", noShift, open)).toEqual({ type: "choose" });
  });

  it("does nothing on Enter or Tab while there are no results (empty or still loading), and never sends", () => {
    const empty = { open: true, optionsCount: 0, selected: 0 };
    expect(decideComposerKey("Enter", noShift, empty)).toEqual({ type: "suppress" });
    expect(decideComposerKey("Tab", noShift, empty)).toEqual({ type: "suppress" });
  });

  it("closes the picker on Escape; a second Escape (already closed) is unhandled", () => {
    const open = { open: true, optionsCount: 2, selected: 0 };
    expect(decideComposerKey("Escape", noShift, open)).toEqual({ type: "close" });
    expect(decideComposerKey("Escape", noShift, closed)).toEqual({ type: "none" });
  });
});
