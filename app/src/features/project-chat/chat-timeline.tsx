import { useEffect, useRef, useState } from "react";
import { ArrowDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { TaskOwner } from "@/features/ai/queries/use-task-ownership";
import type { Names } from "@/features/project-chat/activity";
import { AssistantTurn } from "@/features/project-chat/chat-timeline/assistant-turn";
import { UserMessage } from "@/features/project-chat/chat-timeline/user-message";
import type { MentionTask } from "@/features/project-chat/mentions";
import type { ChatTurn } from "@/features/project-chat/types";

type ChatTimelineProps = {
  turns: ChatTurn[];
  names: Names;
  tasks: MentionTask[];
  owners: Map<number, TaskOwner>;
};

// Close enough to the bottom to keep following new output.
const NEAR_BOTTOM = 120;

export function ChatTimeline({ turns, names, tasks, owners }: ChatTimelineProps) {
  const scroller = useRef<HTMLDivElement>(null);
  const [following, setFollowing] = useState(true);
  const last = turns.at(-1);
  // Changes whenever something new shows: a turn, output, a work step.
  const revision = `${turns.length}:${last?.status}:${last?.output.length}:${last?.activity?.length}:${last?.activity?.at(-1)?.status}`;

  useEffect(() => {
    const element = scroller.current;
    if (element && following) element.scrollTop = element.scrollHeight;
  }, [revision, following]);

  const scrollToEnd = () => {
    const element = scroller.current;
    if (!element) return;
    element.scrollTo({ top: element.scrollHeight, behavior: "smooth" });
    setFollowing(true);
  };

  return (
    <div className="relative min-h-0 flex-1">
      <div
        ref={scroller}
        role="log"
        aria-label="Conversation"
        className="h-full overflow-y-auto"
        onScroll={(event) => {
          const el = event.currentTarget;
          setFollowing(el.scrollHeight - el.scrollTop - el.clientHeight < NEAR_BOTTOM);
        }}
      >
        {/* Bottom padding leaves room for the composer floating over the end. */}
        <div className="mx-auto flex w-full max-w-3xl flex-col gap-6 px-3 pt-4 pb-44 sm:px-4">
          {turns.map((turn) => (
            <div key={turn.id} className="flex flex-col gap-4">
              <UserMessage turn={turn} tasks={tasks} owners={owners} />
              <AssistantTurn turn={turn} names={names} tasks={tasks} owners={owners} />
            </div>
          ))}
        </div>
      </div>
      {!following && (
        <Button
          variant="outline"
          size="sm"
          onClick={scrollToEnd}
          className="absolute bottom-40 left-1/2 -translate-x-1/2 rounded-full bg-background/80 shadow-sm backdrop-blur"
        >
          <ArrowDown />
          Scroll to end
        </Button>
      )}
    </div>
  );
}
