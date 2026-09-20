import { describe, expect, it } from "vitest";
import {
  filterMentions,
  insertMention,
  mentionAt,
  referencedTasks,
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
});
