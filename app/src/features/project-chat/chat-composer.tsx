import { useId, useRef, useState } from "react";
import { Link } from "@tanstack/react-router";
import { ArrowUp, Hash, Square } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { useAISettings } from "@/features/ai/queries/use-ai";
import { useTaskOwnership } from "@/features/ai/queries/use-task-ownership";
import { decideComposerKey } from "@/features/project-chat/chat-composer/composer-keys";
import { MentionPicker } from "@/features/project-chat/chat-composer/mention-picker";
import {
  filterMentions,
  insertMention,
  mentionAt,
  referencedTasks,
  type MentionTask,
} from "@/features/project-chat/mentions";
import {
  useCancelTurnMutation,
  useSendChatMessageMutation,
} from "@/features/project-chat/queries/use-project-chats";
import type { ChatTurn } from "@/features/project-chat/types";
import { useComposerDraft } from "@/features/project-chat/use-composer-draft";

type ChatComposerProps = {
  projectId: number;
  // Undefined for a new chat, which the first message creates.
  chatId: number | undefined;
  tasks: MentionTask[] | undefined;
  tasksLoading: boolean;
  activeTurn: ChatTurn | undefined;
  onSent: (turn: ChatTurn) => void;
};

// Small, rounded, and floating. Enter sends, Shift+Enter starts a new line,
// and # references a task.
export function ChatComposer({
  projectId,
  chatId,
  tasks,
  tasksLoading,
  activeTurn,
  onSent,
}: ChatComposerProps) {
  const { message, setMessage, keyFor, sent } = useComposerDraft(
    `aycorn-project-chat-draft-${projectId}-${chatId ?? "new"}`,
  );
  const settings = useAISettings();
  const ownership = useTaskOwnership(projectId);
  const send = useSendChatMessageMutation(projectId, chatId);
  const cancel = useCancelTurnMutation(projectId, chatId);
  const input = useRef<HTMLTextAreaElement>(null);
  const pickerId = useId();
  const [caret, setCaret] = useState(message.length);
  const [choice, setChoice] = useState(0);
  const [dismissed, setDismissed] = useState(false);

  const mention = mentionAt(message, caret);
  const options = mention ? filterMentions(tasks ?? [], mention).slice(0, 20) : [];
  const pickerOpen = !!mention && !dismissed;
  const selected = Math.min(choice, Math.max(0, options.length - 1));
  const owners = new Map((ownership.data ?? []).map((owner) => [owner.taskId, owner.name]));
  const engine = settings.data?.engine;
  const modelLabel =
    settings.data?.settings.harness === "codex" ? "GPT-6 Sol" : settings.data?.settings.model;
  const blocked = !message.trim() || send.isPending || !!activeTurn || !engine?.ready;

  const focusAt = (position: number) =>
    requestAnimationFrame(() => {
      input.current?.focus();
      input.current?.setSelectionRange(position, position);
    });

  const submit = () => {
    if (blocked) return;
    const text = message;
    send.mutate(
      {
        message: text,
        key: keyFor(text),
        taskIds: referencedTasks(text, tasks ?? []).map((task) => task.id),
      },
      {
        onSuccess: (turn) => {
          sent(text);
          setDismissed(true);
          onSent(turn);
          input.current?.focus();
        },
        onError: (error) => toast.error(error.message),
      },
    );
  };

  const choose = (task: MentionTask) => {
    if (!mention) return;
    const next = insertMention(message, mention, task.id);
    setMessage(next.text);
    setCaret(next.caret);
    setDismissed(true);
    focusAt(next.caret);
  };

  const insertHash = () => {
    const start = input.current?.selectionStart ?? message.length;
    const before = message.slice(0, start);
    const insert = (before && !/\s$/.test(before) ? " " : "") + "#";
    setMessage(before + insert + message.slice(start));
    setCaret(start + insert.length);
    setDismissed(false);
    setChoice(0);
    focusAt(start + insert.length);
  };

  return (
    <div className="relative mx-auto w-full max-w-3xl">
      {pickerOpen && mention && (
        <MentionPicker
          id={pickerId}
          options={options}
          selected={selected}
          mode={mention.mode}
          loading={tasksLoading}
          owners={owners}
          onChoose={choose}
        />
      )}
      <div className="rounded-3xl border border-border/70 bg-card/85 shadow-lg shadow-black/5 backdrop-blur transition-colors focus-within:border-ring/50">
        <Textarea
          ref={input}
          autoFocus
          aria-label="Message Chatter"
          role="combobox"
          aria-autocomplete="list"
          aria-expanded={pickerOpen}
          aria-controls={pickerOpen ? pickerId : undefined}
          aria-activedescendant={
            pickerOpen && options[selected] ? `${pickerId}-${options[selected].id}` : undefined
          }
          value={message}
          maxLength={32000}
          rows={1}
          placeholder="Ask about the project, or # a task"
          className="max-h-52 min-h-12 resize-none rounded-3xl border-0 bg-transparent px-4 pt-3.5 pb-1 shadow-none focus-visible:ring-0 dark:bg-transparent"
          onChange={(event) => {
            setMessage(event.target.value);
            setCaret(event.target.selectionStart);
            setChoice(0);
            setDismissed(false);
          }}
          onSelect={(event) => setCaret(event.currentTarget.selectionStart)}
          onKeyDown={(event) => {
            // Keep app shortcuts out of the message.
            event.stopPropagation();
            if (event.nativeEvent.isComposing) return;
            const action = decideComposerKey(
              event.key,
              { shift: event.shiftKey },
              { open: pickerOpen, optionsCount: options.length, selected },
            );
            switch (action.type) {
              case "close":
                event.preventDefault();
                setDismissed(true);
                break;
              case "move":
                event.preventDefault();
                setChoice(action.index);
                break;
              case "choose":
                event.preventDefault();
                choose(options[selected]);
                break;
              case "suppress":
                event.preventDefault();
                break;
              case "submit":
                event.preventDefault();
                submit();
                break;
              case "none":
                break;
            }
          }}
        />
        <div className="flex items-center gap-1 px-2.5 pb-2.5">
          <Button
            variant="ghost"
            size="icon-sm"
            className="rounded-full text-muted-foreground"
            aria-label="Reference a task"
            onClick={insertHash}
          >
            <Hash />
          </Button>
          {modelLabel && <span className="px-1 text-xs text-muted-foreground">{modelLabel}</span>}
          <div className="ml-auto">
            {activeTurn ? (
              <Button
                size="icon"
                variant="destructive"
                className="size-8 rounded-full"
                aria-label="Stop"
                disabled={cancel.isPending || activeTurn.status === "canceling"}
                onClick={() => cancel.mutate(activeTurn.id)}
              >
                <Square className="size-3 fill-current" />
              </Button>
            ) : (
              <Button
                size="icon"
                className="size-8 rounded-full"
                aria-label="Send"
                disabled={blocked}
                onClick={submit}
              >
                <ArrowUp />
              </Button>
            )}
          </div>
        </div>
      </div>
      {settings.data && !engine?.ready && (
        <p className="mt-2 px-4 text-xs text-muted-foreground">
          {engine?.error}{" "}
          <Link to="/app/settings" search={{ tab: "ai" }} className="underline underline-offset-4">
            AI settings
          </Link>
        </p>
      )}
    </div>
  );
}
