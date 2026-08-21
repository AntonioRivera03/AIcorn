import type { Value } from "platejs";

import { MarkdownPlugin } from "@platejs/markdown";
import { describe, expect, it } from "vitest";

import { createMarkdownEditor } from "@/features/editor/markdown-editor";

// A fresh editor per call, exactly like scripts/md-convert.ts does per batch, so
// no test can be affected by state another test left behind.
const markdownApi = () =>
  createMarkdownEditor().getApi(MarkdownPlugin).markdown;

const toBody = (markdown: string): Value => markdownApi().deserialize(markdown);

const toMarkdown = (value: Value): string => markdownApi().serialize({ value });

// The serializer's own formatting is the canonical form, so a second pass over
// it must be a no-op. Anything that survives this survives a real MCP
// read -> edit -> write cycle.
const roundTrip = (markdown: string): string => toMarkdown(toBody(markdown));

const expectStable = (markdown: string) => {
  const once = roundTrip(markdown);
  expect(roundTrip(once)).toBe(once);
  return once;
};

describe("mdxExpressionRules", () => {
  // remarkMdx claims bare `{...}` as a JSX expression and no node maps to one,
  // so these rules exist to keep the literal text. Plain prose survives even
  // without them on the current @platejs/markdown; the marked case below is
  // where the bug actually bit.
  it("keeps bare braces inside a paragraph", () => {
    const value = toBody('Use {braces} and {"json": 1} here.');

    expect(value).toEqual([
      { children: [{ text: 'Use {braces} and {"json": 1} here.' }], type: "p" },
    ]);
  });

  it("keeps a brace expression that stands alone as its own block", () => {
    const value = toBody('{ "json": 1 }');

    expect(value).toEqual([
      { children: [{ text: '{ "json": 1 }' }], type: "p" },
    ]);
  });

  // Regression: without mdxTextExpression the braced run is dropped outright —
  // `**bold {x} text**` deserialized to "bold " + " text" and `{x}` was gone.
  it("preserves marks on text surrounding a brace expression", () => {
    const value = toBody("**bold {x} text**");

    expect(value).toEqual([
      {
        children: [
          { bold: true, text: "bold " },
          { bold: true, text: "{x}" },
          { bold: true, text: " text" },
        ],
        type: "p",
      },
    ]);
  });

  it("round-trips braces through the escaped form", () => {
    const once = roundTrip("Use {braces} here.");

    // Serializing escapes the brace so remarkMdx does not reclaim it on the
    // way back in; the escape is invisible to the reader and re-parses to the
    // same characters.
    expect(once).toBe("Use \\{braces} here.\n");
    expect(toBody(once)).toEqual([
      { children: [{ text: "Use {braces} here." }], type: "p" },
    ]);
  });
});

describe("toggle rule", () => {
  const toggle: Value = [{ children: [{ text: "Summary text" }], type: "toggle" }];

  it("serializes a toggle as an mdx flow element", () => {
    expect(toMarkdown(toggle)).toBe("<toggle>\n  Summary text\n</toggle>\n");
  });

  it("deserializes a toggle back to a toggle node", () => {
    expect(toBody("<toggle>\n  Summary text\n</toggle>\n")).toEqual(toggle);
  });

  it("unwraps the summary paragraph markdown wraps the children in", () => {
    const [node] = toBody("<toggle>\n  Summary text\n</toggle>\n");

    // A toggle's children are its summary line and are inline-only. Markdown
    // hands the content back as a paragraph; leaving it wrapped would produce a
    // shape the editor never creates itself.
    expect(node.children).toEqual([{ text: "Summary text" }]);
  });

  it("keeps inline marks in the summary", () => {
    const value = toBody("<toggle>\n  Plain **bold**\n</toggle>\n");

    expect(value).toEqual([
      {
        children: [{ text: "Plain " }, { bold: true, text: "bold" }],
        type: "toggle",
      },
    ]);
  });

  it("gives an empty toggle an empty text child", () => {
    const empty: Value = [{ children: [{ text: "" }], type: "toggle" }];

    expect(toBody(toMarkdown(empty))).toEqual(empty);
  });

  it("preserves extra props on the toggle node as attributes", () => {
    const value: Value = [
      { children: [{ text: "Summary" }], collapsed: true, type: "toggle" },
    ];

    expect(toBody(toMarkdown(value))).toEqual(value);
  });

  it("un-nests the toggle's body blocks", () => {
    const value: Value = [
      { children: [{ text: "Summary" }], type: "toggle" },
      { children: [{ text: "Body" }], indent: 1, type: "p" },
    ];

    // Markdown has no representation for a block nested under a toggle, so the
    // body survives as an ordinary sibling paragraph without its indent. This
    // is a known, accepted lossy edge of the format — assert it so a change in
    // the behaviour is a deliberate one.
    expect(toBody(toMarkdown(value))).toEqual([
      { children: [{ text: "Summary" }], type: "toggle" },
      { children: [{ text: "Body" }], type: "p" },
    ]);
  });
});

// These node types were already handled by @platejs/markdown's default rules
// before the toggle/mdx rules were added. They are covered so a dependency bump
// that regresses a default rule fails here rather than silently mangling task
// bodies.
describe("default rules", () => {
  it("round-trips headings", () => {
    const markdown = "# One\n\n## Two\n\n### Three\n";

    expect(toBody(markdown)).toEqual([
      { children: [{ text: "One" }], type: "h1" },
      { children: [{ text: "Two" }], type: "h2" },
      { children: [{ text: "Three" }], type: "h3" },
    ]);
    expect(expectStable(markdown)).toBe(markdown);
  });

  it("round-trips marks", () => {
    const value = toBody("**bold** _italic_ `code` ~~struck~~");

    expect(value).toEqual([
      {
        children: [
          { bold: true, text: "bold" },
          { text: " " },
          { italic: true, text: "italic" },
          { text: " " },
          { code: true, text: "code" },
          { text: " " },
          { strikethrough: true, text: "struck" },
        ],
        type: "p",
      },
    ]);
    expect(expectStable("**bold** _italic_ `code` ~~struck~~")).toBe(
      "**bold** _italic_ `code` ~~struck~~\n",
    );
  });

  it("round-trips links", () => {
    const markdown = "See [the docs](https://example.com).\n";

    expect(toBody(markdown)).toEqual([
      {
        children: [
          { text: "See " },
          {
            children: [{ text: "the docs" }],
            type: "a",
            url: "https://example.com",
          },
          { text: "." },
        ],
        type: "p",
      },
    ]);
    expect(expectStable(markdown)).toBe(markdown);
  });

  // Lists are indent-based in this editor rather than nested list elements, so
  // they are the default rule most likely to break on an upgrade.
  it("round-trips bulleted and nested lists", () => {
    const value = toBody("* a\n* b\n  * nested\n");

    expect(value).toEqual([
      { children: [{ text: "a" }], indent: 1, listStyleType: "disc", type: "p" },
      { children: [{ text: "b" }], indent: 1, listStyleType: "disc", type: "p" },
      {
        children: [{ text: "nested" }],
        indent: 2,
        listStyleType: "disc",
        type: "p",
      },
    ]);
    expect(expectStable("* a\n* b\n  * nested\n")).toBe("* a\n* b\n  * nested\n");
  });

  it("round-trips numbered lists", () => {
    const value = toBody("1. one\n2. two\n");

    expect(value).toEqual([
      {
        children: [{ text: "one" }],
        indent: 1,
        listStart: 1,
        listStyleType: "decimal",
        type: "p",
      },
      {
        children: [{ text: "two" }],
        indent: 1,
        listStart: 2,
        listStyleType: "decimal",
        type: "p",
      },
    ]);
    expect(expectStable("1. one\n2. two\n")).toBe("1. one\n2. two\n");
  });

  it("round-trips todo lists with their checked state", () => {
    const value = toBody("* [ ] open\n* [x] done\n");

    expect(value).toEqual([
      {
        checked: false,
        children: [{ text: "open" }],
        indent: 1,
        listStyleType: "todo",
        type: "p",
      },
      {
        checked: true,
        children: [{ text: "done" }],
        indent: 1,
        listStyleType: "todo",
        type: "p",
      },
    ]);
    expect(expectStable("* [ ] open\n* [x] done\n")).toBe(
      "* [ ] open\n* [x] done\n",
    );
  });

  it("round-trips a fenced code block with its language", () => {
    const markdown = '```go\nfmt.Println("hi")\n```\n';

    expect(toBody(markdown)).toEqual([
      {
        children: [
          { children: [{ text: 'fmt.Println("hi")' }], type: "code_line" },
        ],
        lang: "go",
        type: "code_block",
      },
    ]);
    expect(expectStable(markdown)).toBe(markdown);
  });

  it("round-trips a table", () => {
    const markdown = "| a | b |\n| - | - |\n| 1 | 2 |\n";
    const [table] = toBody(markdown);

    expect(table.type).toBe("table");
    expect(table).toEqual({
      children: [
        {
          children: [
            { children: [{ children: [{ text: "a" }], type: "p" }], type: "th" },
            { children: [{ children: [{ text: "b" }], type: "p" }], type: "th" },
          ],
          type: "tr",
        },
        {
          children: [
            { children: [{ children: [{ text: "1" }], type: "p" }], type: "td" },
            { children: [{ children: [{ text: "2" }], type: "p" }], type: "td" },
          ],
          type: "tr",
        },
      ],
      type: "table",
    });
    expect(expectStable(markdown)).toBe(markdown);
  });

  it("round-trips a blockquote", () => {
    const markdown = "> quoted\n";

    expect(toBody(markdown)).toEqual([
      { children: [{ children: [{ text: "quoted" }], type: "p" }], type: "blockquote" },
    ]);
    expect(expectStable(markdown)).toBe(markdown);
  });

  // The default rules for these emit mdxJsx* mdast nodes, which is why
  // MarkdownKit loads remarkMdx at all — without it serializing throws.
  it("round-trips a callout", () => {
    const value: Value = [
      { children: [{ children: [{ text: "Heads up" }], type: "p" }], type: "callout" },
    ];

    expect(toMarkdown(value)).toContain("<callout>");
    expect(toBody(toMarkdown(value))).toEqual(value);
  });
});
