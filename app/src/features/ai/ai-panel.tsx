import { useRef, useState } from "react";
import { useTaskOwnership } from "./queries/use-task-ownership";
import { Sparkles, X, ArrowUpRight } from "lucide-react";
import {
  Drawer,
  DrawerContent,
  DrawerHeader,
  DrawerTitle,
  DrawerDescription,
} from "@/components/ui/drawer";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { TaskSessionComposer } from "./task-session-composer";
import { AIRunCard } from "@/features/ai/ai-run-card";
import {
  useAIContext,
  useAIMutations,
  useAISettings,
} from "@/features/ai/queries/use-ai";
import {
  useAgentJobs,
  isAgentWorking,
  type AgentJob,
} from "@/features/agentJob/queries/useAgentJobs";
import { usePersonasQuery } from "@/features/persona/queries/use-personas-query";
import type { AITask } from "@/features/ai/ai-context";
import type { AIIntent } from "@/features/ai/types";

const intents: { value: AIIntent; label: string; hint: string }[] = [
  { value: "ask", label: "Ask", hint: "Explain something about this task" },
  {
    value: "plan",
    label: "Plan",
    hint: "Outline an approach and acceptance criteria",
  },
  {
    value: "implement",
    label: "Implement",
    hint: "Make a focused change in an isolated worktree",
  },
  {
    value: "review",
    label: "Review",
    hint: "Inspect the linked repository and report findings",
  },
];

export function AIPanel({
  task,
  onClose,
}: {
  task: AITask;
  onClose: () => void;
}) {
  const [intent, setIntent] = useState<AIIntent>("ask");
  const [instruction, setInstruction] = useState("");
  const [presetId, setPresetId] = useState("none");
  const [useRepository, setUseRepository] = useState(false);
  const textarea = useRef<HTMLTextAreaElement>(null);
  const settings = useAISettings();
  const context = useAIContext(task.id);
  const ownership = useTaskOwnership(context.task.data?.ProjectID);
  const owner = ownership.data?.find((item) => item.taskId === task.id);
  const history = useAgentJobs(task.id);
  const { data: presets = [] } = usePersonasQuery();
  const { start, cancel } = useAIMutations(task.id);
  const jobs = [...(history.data?.jobs ?? [])].sort((a, b) => b.id - a.id);
  const active = isAgentWorking(jobs);
  const latestSession = jobs.find((job) => job.request?.taskSession);
  const needsRepo =
    intent === "implement" || intent === "review" || useRepository;
  const ready =
    settings.data?.engine.ready &&
    (presetId !== "none" || !!settings.data.settings.model);
  const repoMissing =
    needsRepo &&
    !context.projects.isPending &&
    !context.projects.isError &&
    !context.project?.RepoPath;
  const canRun =
    ready &&
    !repoMissing &&
    (!needsRepo ||
      (!context.projects.isPending && !context.projects.isError)) &&
    !active &&
    !owner &&
    !ownership.isPending &&
    !ownership.isError &&
    !start.isPending &&
    !context.task.isPending &&
    !context.task.isError &&
    !history.isError &&
    !history.isPending;
  const submit = () => {
    if (!canRun) return;
    start.mutate({
      intent,
      instruction,
      presetId: presetId === "none" ? 0 : Number(presetId),
      useRepository: needsRepo,
    });
  };
  const retry = (job: AgentJob) => {
    setIntent(job.request?.intent ?? "ask");
    setInstruction(job.request?.instruction ?? "");
    setUseRepository(!!job.request?.repoPath);
    setPresetId(
      job.persona && presets.some((p) => p.ID === job.persona)
        ? String(job.persona)
        : "none",
    );
    textarea.current?.focus();
    textarea.current?.scrollIntoView({ block: "center" });
  };
  return (
    <Drawer
      autoFocus
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      direction="right"
      handleOnly
    >
      <DrawerContent
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          textarea.current?.focus();
        }}
        className="data-[vaul-drawer-direction=right]:w-[min(44rem,100vw)] data-[vaul-drawer-direction=right]:sm:max-w-none"
      >
        <DrawerHeader className="border-b border-border p-5">
          <div className="flex items-center justify-between">
            <DrawerTitle className="flex items-center gap-2">
              <Sparkles className="size-4 text-primary" /> Task AI
            </DrawerTitle>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label="Close AI panel"
              onClick={onClose}
            >
              <X />
            </Button>
          </div>
          <DrawerDescription className="break-words">
            {task.name || "Untitled task"}
          </DrawerDescription>
        </DrawerHeader>
        <div
          className="min-h-0 flex-1 overflow-y-auto p-5 space-y-6"
          data-vaul-no-drag
        >
          {!latestSession && (
            <section aria-label="New AI request" className="space-y-4">
              <div
                className="grid grid-cols-4 gap-1 rounded-lg bg-muted p-1"
                aria-label="Intent"
              >
                {intents.map((item) => (
                  <Button
                    key={item.value}
                    variant={intent === item.value ? "secondary" : "ghost"}
                    className="px-1"
                    size="sm"
                    aria-pressed={intent === item.value}
                    onClick={() => setIntent(item.value)}
                  >
                    {item.label}
                  </Button>
                ))}
              </div>
              <div className="space-y-2">
                <Label htmlFor="ai-instruction">
                  What would you like help with?
                </Label>
                <Textarea
                  ref={textarea}
                  id="ai-instruction"
                  value={instruction}
                  onChange={(e) => setInstruction(e.target.value)}
                  maxLength={32000}
                  placeholder={
                    intents.find((item) => item.value === intent)?.hint
                  }
                  className="min-h-28 resize-y"
                  onKeyDown={(e) => {
                    if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
                      e.preventDefault();
                      submit();
                    }
                  }}
                />
              </div>
              <div className="rounded-lg border border-border p-3 space-y-2 text-xs text-muted-foreground">
                <p className="font-medium text-foreground">
                  Context: this task and its description
                </p>
                <label className="flex items-start gap-2">
                  <input
                    type="checkbox"
                    className="mt-0.5 accent-primary"
                    checked={needsRepo}
                    disabled={intent === "implement" || intent === "review"}
                    onChange={(e) => setUseRepository(e.target.checked)}
                  />{" "}
                  Include linked repository
                  {context.project?.RepoPath
                    ? ` · ${context.project.Name}`
                    : ""}
                </label>
                {intent === "implement" && (
                  <p>
                    Codex can edit files and run project commands and tests in a
                    separate worktree. Command network access is disabled.
                  </p>
                )}
              </div>
              {presets.length > 0 && (
                <div className="flex items-center gap-3">
                  <Label
                    htmlFor="ai-agent"
                    className="shrink-0 text-xs text-muted-foreground"
                  >
                    Agent
                  </Label>
                  <Select value={presetId} onValueChange={setPresetId}>
                    <SelectTrigger id="ai-agent" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="none">Default model</SelectItem>
                      {presets
                        .filter(
                          (preset) =>
                            !["conductor", "chatter"].includes(
                              preset.BuiltinRole ?? "",
                            ),
                        )
                        .map((preset) => (
                          <SelectItem key={preset.ID} value={String(preset.ID)}>
                            {preset.Name || "Untitled agent"}
                          </SelectItem>
                        ))}
                    </SelectContent>
                  </Select>
                </div>
              )}
              {(!ready || settings.isError) && !settings.isPending && (
                <p role="status" className="text-sm text-muted-foreground">
                  {settings.error?.message ||
                    settings.data?.engine.error ||
                    "Choose a model to start using AI."}{" "}
                  <a className="text-primary underline" href="/personas">
                    Open AI settings
                  </a>
                </p>
              )}
              {needsRepo && context.projects.isError && (
                <p role="alert" className="text-sm text-destructive">
                  Could not load repository context.{" "}
                  <Button
                    variant="link"
                    onClick={() => void context.projects.refetch()}
                  >
                    Try again
                  </Button>
                </p>
              )}
              {repoMissing && (
                <p className="text-sm text-muted-foreground">
                  Link a repository in{" "}
                  <a
                    className="text-primary underline"
                    href={`/project/settings/${context.task.data?.ProjectID ?? ""}`}
                  >
                    project settings
                  </a>{" "}
                  to use this context.
                </p>
              )}
              {context.task.isError && (
                <p role="alert" className="text-sm text-destructive">
                  Could not load this task: {context.task.error.message}
                </p>
              )}
              <div className="flex items-center justify-between gap-3">
                <span className="text-xs text-muted-foreground">
                  {ownership.isError
                    ? ownership.error.message
                    : owner
                      ? `${owner.name} is managing this task (${owner.state})`
                      : active
                        ? "One run is already active for this task"
                        : "Ctrl/⌘ Enter to run"}
                </span>
                <Button onClick={submit} disabled={!canRun}>
                  {start.isPending ? "Queuing…" : "Run"}
                  <ArrowUpRight className="size-4" />
                </Button>
              </div>
            </section>
          )}
          <section aria-label="AI run history" className="space-y-3">
            <h2 className="text-sm font-medium">Activity</h2>
            {history.isPending && (
              <p className="text-sm text-muted-foreground" role="status">
                Loading activity…
              </p>
            )}
            {history.isError && (
              <p role="alert" className="text-sm text-destructive">
                {history.error.message}{" "}
                <Button variant="link" onClick={() => void history.refetch()}>
                  Try again
                </Button>
              </p>
            )}
            {!history.isPending && !history.isError && !jobs.length && (
              <p className="py-6 text-center text-sm text-muted-foreground">
                Your answers and changes will appear here.
              </p>
            )}
            {(latestSession ? [...jobs].reverse() : jobs).map((job) => (
              <AIRunCard
                key={job.id}
                job={job}
                run={history.data?.runs.find((run) => run.job === job.id)}
                onStop={() => cancel.mutate(job.id)}
                onRetry={() => retry(job)}
                busy={start.isPending || cancel.isPending}
              />
            ))}
          </section>
          {latestSession && (
            <TaskSessionComposer
              taskId={task.id}
              stage={context.task.data?.Stage}
              latest={latestSession}
              locked={
                active ||
                !!owner ||
                ownership.isPending ||
                ownership.isError ||
                history.isPending ||
                history.isError ||
                context.task.isError
              }
            />
          )}
        </div>
      </DrawerContent>
    </Drawer>
  );
}
