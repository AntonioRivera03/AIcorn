import { useState } from "react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";

// A text field that saves on blur or Enter; Escape puts the saved value back.
// A value problem() flags is shown instead of saved.
export function LinkField({
  id,
  label,
  value,
  placeholder,
  hint,
  problem,
  onSave,
}: {
  id: string;
  label: string;
  value: string;
  placeholder: string;
  hint: string;
  problem: (draft: string) => string | null;
  onSave: (value: string) => void;
}) {
  const [edit, setEdit] = useState<{ base: string; draft: string } | null>(
    null,
  );
  // A draft lasts only while the saved value it was typed over is current, so
  // a save (or a change from elsewhere) shows the server's value.
  const draft = edit?.base === value ? edit.draft : value;
  const issue = problem(draft);

  const commit = () => {
    const trimmed = draft.trim();
    if (trimmed === value || issue) return;
    onSave(trimmed);
  };

  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        value={draft}
        onChange={(e) => setEdit({ base: value, draft: e.target.value })}
        onBlur={commit}
        onKeyDown={(e) => {
          if (e.key === "Enter") commit();
          if (e.key === "Escape") setEdit(null);
        }}
        placeholder={placeholder}
        spellCheck={false}
        autoComplete="off"
        aria-invalid={!!issue}
        aria-describedby={`${id}-hint`}
      />
      <p
        id={`${id}-hint`}
        className={cn(
          "text-xs",
          issue ? "text-destructive" : "text-muted-foreground",
        )}
      >
        {issue ?? hint}
      </p>
    </div>
  );
}
