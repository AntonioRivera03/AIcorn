import { ChevronRight } from "lucide-react";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import type { AISettingsResponse } from "@/features/ai/types";

// Read-only: the program path is resolved on the server, and the time limit
// is a server default.
export function AdvancedSection({ data }: { data: AISettingsResponse }) {
  const { settings, engine } = data;
  const minutes = Math.round(settings.timeoutSeconds / 60);

  return (
    <Collapsible className="flex flex-col gap-3">
      <CollapsibleTrigger className="group flex w-fit items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
        <ChevronRight className="size-4 transition-transform group-data-[state=open]:rotate-90" />
        Advanced
      </CollapsibleTrigger>
      <CollapsibleContent>
        <dl className="grid gap-x-6 gap-y-3 text-sm sm:grid-cols-[max-content_1fr]">
          {settings.harness === "codex" && (
            <>
              <dt className="text-muted-foreground">Codex program</dt>
              <dd className="break-all font-mono text-xs leading-5">
                {settings.executable ||
                  engine.executable ||
                  "Found automatically on the server"}
              </dd>
            </>
          )}
          <dt className="text-muted-foreground">Run time limit</dt>
          <dd>
            {minutes >= 1 ? `${minutes} min` : `${settings.timeoutSeconds} s`}{" "}
            <span className="text-muted-foreground">
              · a run that takes longer is stopped
            </span>
          </dd>
        </dl>
      </CollapsibleContent>
    </Collapsible>
  );
}
