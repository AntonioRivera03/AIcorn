export type ComposerPickerState = {
  open: boolean;
  optionsCount: number;
  selected: number;
};

export type ComposerKeyAction =
  | { type: "none" } // let the browser do its default thing
  | { type: "suppress" } // a picker key with nothing to do; swallow it
  | { type: "move"; index: number }
  | { type: "choose" }
  | { type: "close" }
  | { type: "submit" };

// Pure decision for a composer keydown, kept out of the DOM so it's testable
// headlessly. While the # picker is open, Enter and Tab never send: Enter
// inserts the highlighted task, or does nothing if there isn't one (no
// matches yet, or still loading). Escape closes the picker and nothing else;
// a second Escape (picker already closed) falls through to "none" so the
// browser's own Escape behavior still runs.
export function decideComposerKey(
  key: string,
  modifiers: { shift: boolean },
  picker: ComposerPickerState,
): ComposerKeyAction {
  if (picker.open) {
    if (key === "Escape") return { type: "close" };
    if (key === "ArrowDown" || key === "ArrowUp") {
      if (!picker.optionsCount) return { type: "none" };
      const delta = key === "ArrowDown" ? 1 : picker.optionsCount - 1;
      return { type: "move", index: (picker.selected + delta) % picker.optionsCount };
    }
    if (key === "Enter" || key === "Tab") {
      return picker.optionsCount ? { type: "choose" } : { type: "suppress" };
    }
  }
  if (key === "Enter" && !modifiers.shift) return { type: "submit" };
  return { type: "none" };
}
