import type { ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import { AudioLines, Filter, Pause, Play, Settings2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { useBoardConductor } from "./conductor-context";

export function ConductorControls({ projectId, only, onOnlyChange }: { projectId: number; only: boolean; onOnlyChange: (value: boolean) => void }) {
  const conductor = useBoardConductor();
  const data = conductor?.data;
  return <div className="flex flex-wrap items-center gap-1.5">
    <Button variant={only ? "secondary" : "ghost"} size="sm" aria-pressed={only} onClick={() => onOnlyChange(!only)}>
      <Filter className="size-3.5" /><span>Conductor<span className="hidden sm:inline"> tasks</span>{data ? ` · ${data.tasks.length}` : ""}</span>
    </Button>
    <Button variant="outline" size="sm" className={cn(data?.settings.enabled && "border-conductor/40 text-conductor")} disabled={!data || conductor?.update.isPending || (!!data.configurationError && !data.settings.enabled)} onClick={() => conductor?.update.mutate({ enabled: !data?.settings.enabled })}>
      {data?.settings.enabled ? <Pause className="size-3.5" /> : <Play className="size-3.5" />}
      {data?.settings.enabled ? "Pause Conductor" : "Start Conductor"}
    </Button>
    <Button variant="ghost" size="icon-sm" asChild>
      <Link to="/project/settings/$projectId" params={{ projectId: String(projectId) }} search={{ tab: "conductor" }} aria-label="Conductor settings"><Settings2 className="size-4" /></Link>
    </Button>
    {conductor?.isError && <span role="alert" className="text-xs text-destructive">Conductor is unavailable. <button className="underline" onClick={() => void conductor.refetch()}>Retry</button></span>}
    {data?.configurationError && !data.settings.enabled && <span className="text-xs text-muted-foreground">Choose stages in Conductor settings.</span>}
  </div>;
}

export function ConductorFrame({ children, only }: { children: ReactNode; only: boolean }) {
  const conductor = useBoardConductor();
  const data = conductor?.data;
  const enabled = data?.settings.enabled;
  const tasks = data?.tasks ?? [];
  const count = (states: string[]) => tasks.filter((t) => states.includes(t.state)).length;
  const visible = enabled || tasks.length > 0 || only;
  return <section aria-label={enabled ? "Conductor managed board" : "Project tasks"} className={cn("flex h-full min-h-0 flex-col gap-3", visible && "rounded-2xl border border-border p-3 sm:p-4", enabled && "conductor-frame")}>
    {visible && <div className="flex flex-wrap items-center gap-x-5 gap-y-2 text-xs">
      <span className="inline-flex items-center gap-2 font-semibold tracking-[0.16em] text-conductor"><AudioLines className={cn("size-4", enabled && "animate-pulse motion-reduce:animate-none")} />CONDUCTOR {enabled ? "MANAGING" : "PAUSED"}</span>
      <span className="text-muted-foreground">{count(["waiting", "planning"])} planning <span aria-hidden="true">·</span> {count(["queued", "working"])} in progress <span aria-hidden="true">·</span> {count(["completed"])} for review</span>
      {count(["needs_context", "held", "failed"]) > 0 && <span className="text-conductor">{count(["needs_context", "held", "failed"])} need attention</span>}
      <span className="ml-auto text-muted-foreground">{enabled ? "You make the final call." : "New work is paused. Active sessions can finish."}</span>
    </div>}
    {enabled && data?.configurationError && <p role="alert" className="text-sm text-destructive">{data.configurationError}</p>}
    {only && tasks.length === 0 && <p className="rounded-lg border border-dashed border-border p-5 text-sm text-muted-foreground">Send tasks to Conductor from a task’s actions menu, or select several tasks and use the toolbar.</p>}
    {children}
  </section>;
}
