export type MentionTask = { id: number; title: string };
export type Mention = {
  start: number;
  end: number;
  query: string;
  mode: "id" | "title";
};
export function mentionAt(text: string, caret: number): Mention | null {
  const prefix = text.slice(0, caret);
  const match = /(?:^|\s)#(?:(\d*)|"([^"\n]*))$/.exec(prefix);
  if (!match) return null;
  const start = prefix.lastIndexOf("#");
  const title = match[2] !== undefined;
  let end = caret;
  if (title) {
    const suffix = /^[^"\n]*"/.exec(text.slice(caret));
    if (suffix) end += suffix[0].length;
  } else end += /^\d*/.exec(text.slice(caret))![0].length;
  return {
    start,
    end,
    query: title ? match[2] : match[1],
    mode: title ? "title" : "id",
  };
}
export function filterMentions(tasks: MentionTask[], mention: Mention) {
  return tasks.filter((task) =>
    mention.mode === "id"
      ? String(task.id).startsWith(mention.query)
      : task.title
          .toLocaleLowerCase()
          .includes(mention.query.toLocaleLowerCase()),
  );
}
export function insertMention(text: string, mention: Mention, id: number) {
  const token = `#${id} `;
  return {
    text: text.slice(0, mention.start) + token + text.slice(mention.end),
    caret: mention.start + token.length,
  };
}
export function referencedTasks(text: string, tasks: MentionTask[]) {
  const ids = new Set(
    Array.from(text.matchAll(/(?:^|[^A-Za-z0-9_])#(\d+)\b/g), (match) =>
      Number(match[1]),
    ),
  );
  return tasks.filter((task) => ids.has(task.id));
}
