import { renderToStaticMarkup } from "react-dom/server";
import { expect, it } from "vitest";
import { AIMarkdown } from "./ai-markdown";

it("keeps native file citations from navigating to nonexistent application routes", () => {
  const html = renderToStaticMarkup(<AIMarkdown>{"Edited [hello.txt](/tmp/chat/hello.txt) and [source](src/main.go)."}</AIMarkdown>);
  expect(html).not.toContain("href=");
  expect(html).toContain('title="/tmp/chat/hello.txt"');
  expect(html).toContain("hello.txt</code>");
});

it("retains web sources and does not execute agent-provided markup or URLs", () => {
  const html = renderToStaticMarkup(<AIMarkdown>{'[Docs](https://example.com/docs) [bad](javascript:alert%281%29) <script>alert(1)</script>'}</AIMarkdown>);
  expect(html).toContain('href="https://example.com/docs"');
  expect(html).toContain('rel="noopener noreferrer"');
  expect(html).not.toContain('href="javascript:');
  expect(html).not.toContain('<script>');
});
