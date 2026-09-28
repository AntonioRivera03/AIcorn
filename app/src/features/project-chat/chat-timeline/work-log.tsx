import type { Names } from "@/features/project-chat/activity";
import { ThinkingRow } from "@/features/project-chat/chat-timeline/thinking-row";
import { ToolGroup } from "@/features/project-chat/chat-timeline/tool-group";
import { ToolRow } from "@/features/project-chat/chat-timeline/tool-row";
import type { ChatActivity } from "@/features/project-chat/types";

type Segment =
  | { kind: "thinking"; activity: ChatActivity }
  | { kind: "tools"; activity: ChatActivity[] };

const segments = (activity: ChatActivity[]) =>
  activity.reduce<Segment[]>((out, a) => {
    const last = out.at(-1);
    if (a.kind === "tool" && last?.kind === "tools") last.activity.push(a);
    else if (a.kind === "tool") out.push({ kind: "tools", activity: [a] });
    else out.push({ kind: "thinking", activity: a });
    return out;
  }, []);

// A turn's thinking and tool calls in order. While live every call shows;
// afterwards runs of calls fold into one line each.
export function WorkLog({
  activity,
  names,
  live,
}: {
  activity: ChatActivity[];
  names: Names;
  live: boolean;
}) {
  return (
    <div className="flex flex-col gap-0.5">
      {segments(activity).map((segment) =>
        segment.kind === "thinking" ? (
          <ThinkingRow key={segment.activity.id} activity={segment.activity} />
        ) : live || segment.activity.length === 1 ? (
          segment.activity.map((a) => <ToolRow key={a.id} activity={a} names={names} />)
        ) : (
          <ToolGroup key={segment.activity[0].id} activity={segment.activity} names={names} />
        ),
      )}
    </div>
  );
}
