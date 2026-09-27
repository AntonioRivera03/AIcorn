import type { LucideIcon } from "lucide-react";
import { ChevronRight } from "lucide-react";

type Props = {
  icon: LucideIcon;
  title: string;
  description: string;
  disabled?: boolean;
  autoFocus?: boolean;
  onSelect: () => void;
};

export function UsageOption({ icon: Icon, title, description, disabled, autoFocus, onSelect }: Props) {
  return (
    <button
      type="button"
      onClick={onSelect}
      disabled={disabled}
      autoFocus={autoFocus}
      className="group flex w-full items-center gap-4 rounded-lg border bg-card p-4 text-left transition-colors hover:bg-accent/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
    >
      <span className="flex size-9 shrink-0 items-center justify-center rounded-md border bg-background">
        <Icon className="size-4" />
      </span>
      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className="text-sm font-medium">{title}</span>
        <span className="text-xs text-muted-foreground">{description}</span>
      </span>
      <ChevronRight className="size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
    </button>
  );
}
