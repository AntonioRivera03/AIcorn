import { useState } from "react";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";

type InstructionsFieldProps = {
  value: string;
  onSave: (value: string) => void;
};

// Saves on blur. The parent keys it by the saved value, so a change from the
// server resets the draft.
export function InstructionsField({ value, onSave }: InstructionsFieldProps) {
  const [draft, setDraft] = useState(value);

  return (
    <div className="flex flex-col gap-2">
      <Label htmlFor="conductor-instructions">Instructions for Conductor</Label>
      <Textarea
        id="conductor-instructions"
        className="min-h-24 resize-y"
        value={draft}
        placeholder="For example: start bugs before features, and hold back anything tagged design."
        onChange={(event) => setDraft(event.target.value)}
        onBlur={() => {
          if (draft !== value) onSave(draft);
        }}
        onKeyDown={(event) => event.stopPropagation()}
      />
      <p className="text-xs text-muted-foreground">
        Optional. How Conductor should choose which tasks to start and which to
        hold back.
      </p>
    </div>
  );
}
