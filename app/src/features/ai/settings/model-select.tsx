import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useHarnessModels } from "@/features/ai/queries/use-ai";
import type { PersonaHarness } from "@/types/types";

// Radix Select can't use "" as an item value, so "use the default model" gets
// its own token and maps back to "" for the API.
const DEFAULT_VALUE = "__default__";

type ModelSelectProps = {
  id?: string;
  ariaLabel?: string;
  harness: PersonaHarness;
  value: string;
  onChange: (model: string) => void;
  disabled?: boolean;
  // When set, the list starts with a "Default" option that saves as "".
  defaultLabel?: string;
  className?: string;
};

export function ModelSelect({
  id,
  ariaLabel,
  harness,
  value,
  onChange,
  disabled,
  defaultLabel,
  className,
}: ModelSelectProps) {
  const models = useHarnessModels(harness);
  const options = models.data?.models ?? [];
  // A saved model the harness no longer lists stays visible instead of
  // silently showing an empty trigger.
  const unlisted =
    value !== "" && !options.some((model) => model.id === value);

  return (
    <Select
      value={value === "" ? DEFAULT_VALUE : value}
      onValueChange={(next) => onChange(next === DEFAULT_VALUE ? "" : next)}
      disabled={disabled || models.isPending}
    >
      <SelectTrigger id={id} aria-label={ariaLabel} className={className}>
        <SelectValue
          placeholder={models.isPending ? "Loading models…" : "Choose a model"}
        />
      </SelectTrigger>
      <SelectContent>
        {defaultLabel !== undefined && (
          <SelectItem value={DEFAULT_VALUE}>{defaultLabel}</SelectItem>
        )}
        {unlisted && (
          <SelectItem value={value}>{value} · not offered</SelectItem>
        )}
        {options.map((model) => (
          <SelectItem key={model.id} value={model.id}>
            {model.name}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
