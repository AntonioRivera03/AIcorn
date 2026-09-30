import { describe, expect, it } from "vitest";
import {
  filterMentions,
  insertMention,
  keepTaskLinks,
  linkifyMarkdownMentions,
  mentionAt,
  referencedTasks,
  splitMentionSegments,
} from "./mentions";
const tasks = [
  { id: 12, title: "Fix login" },
  { id: 123, title: "Export report" },
  { id: 2, title: "Login page" },
];
describe("project task mentions", () => {
  it("filters numbers and quoted titles at the caret", () => {
    expect(filterMentions(tasks, mentionAt("Work on #12", 11)!)).toEqual(
      tasks.slice(0, 2),
    );
    const text = 'Work on #"LOGIN';
    expect(filterMentions(tasks, mentionAt(text, text.length)!)).toEqual([
      tasks[0],
      tasks[2],
    ]);
    expect(mentionAt("email#12", 8)).toBeNull();
    expect(mentionAt('#"done"', 7)).toBeNull();
  });
  it("replaces the whole number or quoted token without losing following prose", () => {
    const text = 'Use #"Fix login" tomorrow';
    const at = mentionAt(text, 10)!;
    expect(insertMention(text, at, 12).text).toBe("Use #12  tomorrow");
    expect(
      insertMention("Use #123 later", mentionAt("Use #123 later", 6)!, 2).text,
    ).toBe("Use #2  later");
  });
  it("only links known project tasks and deduplicates references", () => {
    expect(referencedTasks("Use **#12** and #12 but not #999", tasks)).toEqual([
      tasks[0],
    ]);
  });
  it("splits plain text into runs and known #id runs for rendering", () => {
    expect(splitMentionSegments("See #12 and #999, plus email#12 and #12x", tasks)).toEqual([
      { type: "text", text: "See " },
      { type: "task", id: 12, text: "#12" },
      { type: "text", text: " and #999, plus email#12 and #12x" },
    ]);
    // No mentions at all: a single plain-text segment, not an empty list.
    expect(splitMentionSegments("Nothing here", tasks)).toEqual([
      { type: "text", text: "Nothing here" },
    ]);
  });
  it("linkifies known #id mentions in markdown but leaves unknown ids, punctuation, and inside-word numbers alone", () => {
    expect(linkifyMarkdownMentions("See #12, #999 and email#12.", tasks)).toBe(
      "See [#12](aycorn-task:12), #999 and email#12.",
    );
    expect(linkifyMarkdownMentions("Not #12x", tasks)).toBe("Not #12x");
  });
  it("never linkifies mentions inside inline code spans or fenced code blocks", () => {
    expect(linkifyMarkdownMentions("Use `#12` for now", tasks)).toBe("Use `#12` for now");
    const fenced = "```\nconst id = 12; // #12\n```\nSee #12";
    expect(linkifyMarkdownMentions(fenced, tasks)).toBe(
      "```\nconst id = 12; // #12\n```\nSee [#12](aycorn-task:12)",
    );
  });
  it("leaves markdown unchanged when no tasks are known", () => {
    expect(linkifyMarkdownMentions("See #12", [])).toBe("See #12");
  });
  it("keeps task links through the markdown URL sanitizer and still blocks unsafe schemes", () => {
    expect(keepTaskLinks("aycorn-task:12")).toBe("aycorn-task:12");
    expect(keepTaskLinks("https://github.com/o/r/pull/1")).toBe("https://github.com/o/r/pull/1");
    expect(keepTaskLinks("javascript:alert(1)")).toBe("");
  });
});
