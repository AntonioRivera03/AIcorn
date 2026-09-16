import { useId, useRef, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Send } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogCancel,
} from "@/components/ui/alert-dialog";
import type {
  AgentJob,
  AgentRun,
} from "@/features/agentJob/queries/useAgentJobs";

type Input = {
  key: string;
  message: string;
  previousJob: number;
  expectedStage: number;
  decision: "auto" | "question" | "work";
};
type Result = {
  job?: AgentJob;
  confirmationRequired: boolean;
  requiresNewSession?: boolean;
  intent: { intent: string; reason: string; provider: string };
};

export function TaskSessionComposer({
  taskId,
  stage,
  latest,
  locked,
}: {
  taskId: number;
  stage?: number;
  latest: AgentJob;
  locked: boolean;
}) {
  const id = useId();
  const client = useQueryClient();
  const [message, setMessage] = useState("");
  const [confirmation, setConfirmation] = useState<{
    input: Input;
    result: Result;
  } | null>(null);
  const pending = useRef<Input | null>(null);
  const send = useMutation({
    mutationFn: async (input: Input): Promise<Result> => {
      const response = await fetch(`/api/ai/tasks/${taskId}/session/messages`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(input),
      });
      if (!response.ok) throw new Error((await response.text()).trim());
      return response.json() as Promise<Result>;
    },
    onSuccess: (result, input) => {
      if (result.confirmationRequired) {
        setConfirmation({ input, result });
        return;
      }
      setConfirmation(null);
      pending.current = null;
      setMessage((draft) => (draft === input.message ? "" : draft));
      if (result.job) {
        const job = result.job;
        client.setQueryData<{ jobs: AgentJob[]; runs: AgentRun[] }>(
          ["agent-jobs", taskId],
          (old) =>
            old
              ? {
                  ...old,
                  jobs: [...old.jobs.filter((j) => j.id !== job.id), job],
                }
              : { jobs: [job], runs: [] },
        );
      }
      for (const key of [
        "agent-jobs",
        "ai-task",
        "conductor",
        "projectDetails",
        "task-ownership",
        "active-agent-jobs",
      ])
        void client.invalidateQueries({ queryKey: [key] });
    },
    onError: () => {
      // Keep the draft and idempotency key if a response was lost.
      for (const key of ["agent-jobs", "ai-task", "task-ownership"])
        void client.invalidateQueries({ queryKey: [key] });
    },
  });
  const blocked =
    locked || send.isPending || !message.trim() || stage === undefined;
  function submit() {
    if (blocked || stage === undefined) return;
    if (
      !pending.current ||
      pending.current.message !== message ||
      pending.current.previousJob !== latest.id ||
      pending.current.expectedStage !== stage
    ) {
      pending.current = {
        key: crypto.randomUUID(),
        message,
        previousJob: latest.id,
        expectedStage: stage,
        decision: "auto",
      };
    }
    send.mutate(pending.current);
  }
  const confirmationStale =
    !!confirmation &&
    (confirmation.input.previousJob !== latest.id ||
      confirmation.input.expectedStage !== stage);
  const confirmBlocked = locked || send.isPending || confirmationStale;
  return (
    <section
      aria-label="Continue task session"
      className="space-y-3 border-t border-border pt-4"
    >
      <Label htmlFor={id}>Continue this task’s conversation</Label>
      <Textarea
        id={id}
        value={message}
        disabled={locked || send.isPending || !!confirmation}
        onChange={(e) => setMessage(e.target.value)}
        maxLength={32000}
        placeholder="Ask about the result or request more work…"
        className="min-h-24 resize-y"
        onKeyDown={(e) => {
          if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
            e.preventDefault();
            submit();
          }
        }}
      />
      <div className="flex items-center justify-between gap-3">
        <p role="status" className="text-xs text-muted-foreground">
          {locked
            ? "The session is working. You can continue when it finishes."
            : "Questions keep the current stage. More work asks for confirmation."}
        </p>
        <Button disabled={blocked} onClick={submit}>
          <Send className="size-4" />
          {send.isPending ? "Sending…" : "Send"}
        </Button>
      </div>
      {send.error && (
        <p role="alert" className="text-sm text-destructive">
          {send.error.message}
        </p>
      )}
      <AlertDialog
        open={!!confirmation}
        onOpenChange={(open) => {
          if (!open && !send.isPending) setConfirmation(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {confirmation?.result.intent.intent === "work"
                ? "Resume work on this task?"
                : "How should the agent respond?"}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {confirmation?.result.intent.reason}{" "}
              {latest.request?.taskSession?.settings
                ? "Resuming work moves the task to In progress and locks this chat until the agent finishes."
                : "Resuming work locks this chat until the agent finishes."}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {confirmationStale && (
            <p role="alert" className="text-sm text-destructive">
              The task changed. Cancel and send your message again.
            </p>
          )}
          {send.error && (
            <p role="alert" className="text-sm text-destructive">
              {send.error.message}
            </p>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={send.isPending}>
              Cancel
            </AlertDialogCancel>
            <Button
              variant="outline"
              disabled={
                confirmBlocked || confirmation?.result.requiresNewSession
              }
              onClick={() => {
                if (confirmation)
                  send.mutate({ ...confirmation.input, decision: "question" });
              }}
            >
              Ask only
            </Button>
            <Button
              disabled={confirmBlocked}
              onClick={() => {
                if (confirmation)
                  send.mutate({ ...confirmation.input, decision: "work" });
              }}
            >
              Resume work
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  );
}
