import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { Stage } from "@/types/types";

type StageSelectProps = {
  id: string;
  label: string;
  hint: string;
  value: number;
  stages: Stage[];
  // The stage picked for the other role, which this one can't share.
  otherValue: number;
  // A running Conductor always needs both stages.
  required: boolean;
  disabled: boolean;
  onChange: (stageId: number) => void;
};

export function StageSelect({
  id,
  label,
  hint,
  value,
  stages,
  otherValue,
  required,
  disabled,
  onChange,
}: StageSelectProps) {
  return (
    <div className="flex flex-col gap-2">
      <Label htmlFor={id}>{label}</Label>
      <Select
        value={value ? String(value) : "none"}
        disabled={disabled}
        onValueChange={(next) => onChange(next === "none" ? 0 : Number(next))}
      >
        <SelectTrigger id={id} className="w-full">
          <SelectValue placeholder="Choose a stage" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="none" disabled={required}>
            Choose a stage
          </SelectItem>
          {stages.map((stage) => (
            <SelectItem
              key={stage.ID}
              value={String(stage.ID)}
              disabled={stage.ID === otherValue}
            >
              {stage.Name || "Untitled stage"}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground">{hint}</p>
    </div>
  );
}
