import type { ChatActivity } from "@/features/project-chat/types";

// Looks up names for the ids in tool calls, from the project's tasks and stages.
export type Names = {
  task: (id: number) => string | undefined;
  stage: (id: number) => string | undefined;
};

export type ToolIcon =
  | "project"
  | "read"
  | "search"
  | "document"
  | "create"
  | "edit"
  | "move"
  | "link"
  | "send"
  | "web"
  | "tool";

export type ToolStep = {
  icon: ToolIcon;
  // "Moved #17 to Review", "Moving #17", "Couldn't move #17".
  label: string;
  // The task a row can link to.
  taskId?: number;
};

export type TaskChange = {
  key: string;
  kind: "created" | "updated" | "moved" | "linked" | "sent";
  taskIds: number[];
  label: string;
};

const record = (value: unknown): Record<string, unknown> =>
  value && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};

const num = (value: unknown) =>
  typeof value === "number" && Number.isFinite(value) ? value : undefined;
const str = (value: unknown) => (typeof value === "string" ? value : undefined);

const plural = (count: number, one: string, many = `${one}s`) =>
  `${count} ${count === 1 ? one : many}`;

const taskRef = (id: number | undefined, names: Names, fallbackTitle?: string) => {
  if (!id) return "a task";
  const title = names.task(id) ?? fallbackTitle;
  return title ? `#${id} ${title}` : `#${id}`;
};

// Verb forms per tool: done, in progress, and the plain form for failures.
type Verb = { done: string; live: string; base: string };
const verb = (done: string, live: string, base: string): Verb => ({ done, live, base });

const phrase = (v: Verb, status: ChatActivity["status"], object: string) =>
  status === "failed"
    ? `Couldn't ${v.base} ${object}`
    : `${status === "inProgress" ? v.live : v.done} ${object}`;

export const describeTool = (activity: ChatActivity, names: Names): ToolStep => {
  const args = record(activity.arguments);
  const result = record(activity.result);
  const status = activity.status;
  const taskId = num(args.taskId);
  switch (activity.tool) {
    case "project_context":
      return { icon: "project", label: phrase(verb("Checked", "Checking", "check"), status, "the project") };
    case "read_task":
      return {
        icon: "read",
        label: phrase(verb("Read", "Reading", "read"), status, taskRef(taskId, names, str(result.Name))),
        taskId,
      };
    case "search_tasks": {
      const query = str(args.query);
      return {
        icon: "search",
        label: phrase(verb("Searched", "Searching", "search"), status, query ? `tasks for “${query}”` : "tasks"),
      };
    }
    case "read_project_document": {
      const title = str(result.title);
      return {
        icon: "document",
        label: phrase(verb("Read", "Reading", "read"), status, title ? `“${title}”` : "a document"),
      };
    }
    case "create_task": {
      const id = num(result.ID);
      const name = str(result.Name) ?? str(args.name);
      return {
        icon: "create",
        label: id
          ? phrase(verb("Created", "Creating", "create"), status, taskRef(id, names, name))
          : phrase(verb("Created", "Creating", "create"), status, name ? `“${name}”` : "a task"),
        taskId: id,
      };
    }
    case "update_task":
      return { icon: "edit", label: phrase(verb("Updated", "Updating", "update"), status, taskRef(taskId, names)), taskId };
    case "move_task_stage": {
      const stage = num(args.toStage) ? names.stage(num(args.toStage)!) : undefined;
      const object = taskRef(taskId, names) + (stage ? ` to ${stage}` : "");
      return { icon: "move", label: phrase(verb("Moved", "Moving", "move"), status, object), taskId };
    }
    case "list_task_links":
      return { icon: "link", label: phrase(verb("Checked links on", "Checking links on", "check links on"), status, taskRef(taskId, names)), taskId };
    case "add_task_link": {
      const kind = str(args.url)?.includes("/pull/") ? "a pull request" : "a branch";
      return { icon: "link", label: phrase(verb("Linked", "Linking", "link"), status, `${kind} to ${taskRef(taskId, names)}`), taskId };
    }
    case "remove_task_link":
      return { icon: "link", label: phrase(verb("Removed a link from", "Removing a link from", "remove a link from"), status, taskRef(taskId, names)), taskId };
    case "send_to_conductor": {
      const ids = Array.isArray(args.taskIds) ? args.taskIds.filter((id): id is number => typeof id === "number") : [];
      const object = ids.length === 1 ? taskRef(ids[0], names) : plural(ids.length, "task");
      return { icon: "send", label: phrase(verb("Sent", "Sending", "send"), status, `${object} to Conductor`), taskId: ids.length === 1 ? ids[0] : undefined };
    }
    case "web_search":
      return { icon: "web", label: phrase(verb("Searched", "Searching", "search"), status, "the web") };
    default:
      return { icon: "tool", label: phrase(verb("Used", "Using", "use"), status, (activity.tool ?? "a tool").replaceAll("_", " ")) };
  }
};

// "Read 3 tasks, searched tasks, and created 1 task", for a folded run of
// tool calls.
export const summarizeTools = (activity: ChatActivity[]) => {
  const count = (tool: string) => activity.filter((a) => a.tool === tool).length;
  const parts: string[] = [];
  const add = (n: number, text: (n: number) => string) => {
    if (n) parts.push(text(n));
  };
  add(count("project_context"), () => "checked the project");
  add(count("read_task"), (n) => `read ${plural(n, "task")}`);
  add(count("read_project_document"), (n) => `read ${plural(n, "document")}`);
  add(count("search_tasks"), (n) => (n === 1 ? "searched tasks" : `searched tasks ${n} times`));
  add(count("create_task"), (n) => `created ${plural(n, "task")}`);
  add(count("update_task"), (n) => `updated ${plural(n, "task")}`);
  add(count("move_task_stage"), (n) => `moved ${plural(n, "task")}`);
  add(count("add_task_link") + count("remove_task_link") + count("list_task_links"), () => "worked on task links");
  add(count("send_to_conductor"), () => "sent work to Conductor");
  const known = new Set([
    "project_context", "read_task", "read_project_document", "search_tasks", "create_task",
    "update_task", "move_task_stage", "add_task_link", "remove_task_link", "list_task_links", "send_to_conductor",
  ]);
  add(activity.filter((a) => !known.has(a.tool ?? "")).length, (n) => `used ${plural(n, "other tool")}`);
  if (!parts.length) return "Used tools";
  const sentence =
    parts.length === 1 ? parts[0] : `${parts.slice(0, -1).join(", ")}${parts.length > 2 ? "," : ""} and ${parts.at(-1)}`;
  return sentence[0].toUpperCase() + sentence.slice(1);
};

// What a turn changed, for the summary under its answer. Several edits to
// one task show once, and a task created in the turn only shows as created.
export const taskChanges = (activity: ChatActivity[], names: Names): TaskChange[] => {
  const changes: TaskChange[] = [];
  const created = new Set<number>();
  const seen = new Set<string>();
  const push = (change: TaskChange) => {
    if (seen.has(change.key)) return;
    seen.add(change.key);
    changes.push(change);
  };
  for (const a of activity) {
    if (a.kind !== "tool" || a.status !== "completed") continue;
    const args = record(a.arguments);
    const result = record(a.result);
    const taskId = num(args.taskId);
    switch (a.tool) {
      case "create_task": {
        const id = num(result.ID);
        if (!id) break;
        created.add(id);
        push({ key: `created:${id}`, kind: "created", taskIds: [id], label: `Created ${taskRef(id, names, str(result.Name))}` });
        break;
      }
      case "update_task":
        if (taskId && !created.has(taskId))
          push({ key: `updated:${taskId}`, kind: "updated", taskIds: [taskId], label: `Updated ${taskRef(taskId, names)}` });
        break;
      case "move_task_stage": {
        if (!taskId) break;
        const stage = num(args.toStage) ? names.stage(num(args.toStage)!) : undefined;
        // A later move replaces an earlier one's label.
        const key = `moved:${taskId}`;
        const label = `Moved ${taskRef(taskId, names)}${stage ? ` to ${stage}` : ""}`;
        const existing = changes.find((change) => change.key === key);
        if (existing) existing.label = label;
        else push({ key, kind: "moved", taskIds: [taskId], label });
        break;
      }
      case "add_task_link":
      case "remove_task_link":
        if (taskId)
          push({ key: `linked:${taskId}`, kind: "linked", taskIds: [taskId], label: `Updated links on ${taskRef(taskId, names)}` });
        break;
      case "send_to_conductor": {
        const ids = Array.isArray(args.taskIds) ? args.taskIds.filter((id): id is number => typeof id === "number") : [];
        const sent = num(result.sent) ?? ids.length;
        if (!sent) break;
        const label =
          ids.length === 1
            ? `Sent ${taskRef(ids[0], names)} to Conductor`
            : `Sent ${sent === ids.length ? plural(sent, "task") : `${sent} of ${ids.length} tasks`} to Conductor`;
        push({ key: `sent:${a.id}`, kind: "sent", taskIds: ids, label });
        break;
      }
    }
  }
  return changes;
};

export const formatDuration = (ms: number) => {
  const seconds = Math.max(1, Math.round(ms / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const rest = seconds % 60;
  return rest ? `${minutes}m ${rest}s` : `${minutes}m`;
};

// The first line of a thinking summary, without Markdown emphasis, for its
// collapsed row.
export const thinkingPreview = (text: string | undefined) =>
  (text ?? "")
    .split("\n")
    .map((line) => line.replace(/[*_`#>]/g, "").trim())
    .find(Boolean) ?? "";
