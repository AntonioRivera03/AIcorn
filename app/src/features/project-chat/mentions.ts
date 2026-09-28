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

// A #123 token that isn't part of a longer word, e.g. not "email#12" or
// "#12abc" — the same rule referencedTasks uses to collect ids.
const mentionIdPattern = /(?<![A-Za-z0-9_])#(\d+)\b/g;

export type MentionSegment =
  | { type: "text"; text: string }
  | { type: "task"; id: number; text: string };

// Splits plain text (the user's own message, never parsed as markdown) into
// runs of text and #id runs naming a real task in `tasks`; other numbers are
// left as plain text.
export function splitMentionSegments(text: string, tasks: MentionTask[]): MentionSegment[] {
  const known = new Set(tasks.map((task) => task.id));
  const segments: MentionSegment[] = [];
  let last = 0;
  for (const match of text.matchAll(mentionIdPattern)) {
    const id = Number(match[1]);
    if (!known.has(id)) continue;
    const start = match.index;
    if (start > last) segments.push({ type: "text", text: text.slice(last, start) });
    segments.push({ type: "task", id, text: match[0] });
    last = start + match[0].length;
  }
  if (last < text.length || segments.length === 0) segments.push({ type: "text", text: text.slice(last) });
  return segments;
}

// A fenced code block (``` or ~~~) or an inline `code` span — skipped so
// mentions inside code are never linked.
const codeSegmentPattern = /```[\s\S]*?```|~~~[\s\S]*?~~~|`[^`\n]*`/g;

// Turns #id into a markdown link (`aycorn-task:<id>`) for every id in
// `tasks`, everywhere except inside code spans/blocks; other numbers are
// left alone. The renderer resolves the link target and any owner badge.
export function linkifyMarkdownMentions(markdown: string, tasks: MentionTask[]): string {
  const known = new Set(tasks.map((task) => task.id));
  if (!known.size) return markdown;
  let out = "";
  let last = 0;
  for (const code of markdown.matchAll(codeSegmentPattern)) {
    out += linkifyMentionRun(markdown.slice(last, code.index), known);
    out += code[0];
    last = code.index + code[0].length;
  }
  return out + linkifyMentionRun(markdown.slice(last), known);
}

function linkifyMentionRun(text: string, known: Set<number>): string {
  return text.replace(mentionIdPattern, (match, digits: string) =>
    known.has(Number(digits)) ? `[${match}](aycorn-task:${digits})` : match,
  );
}
