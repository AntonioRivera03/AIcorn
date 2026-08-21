import type { Value } from "platejs";

import { MarkdownPlugin } from "@platejs/markdown";

import { createMarkdownEditor } from "@/features/editor/markdown-editor";

/**
 * Headless markdown <-> Plate.js JSON converter, driven over stdin/stdout by
 * `server/internal/markdown`. It reads one JSON request, writes one JSON
 * response, and exits — see that package for the Go side of the contract.
 *
 * Requests are batched because process startup dominates the cost: converting
 * 25 search results one spawn at a time would cost 25 Node boots.
 */
type ConvertRequest = {
  direction: "toMarkdown" | "toBody";
  items: string[];
};

// A Plate document is never legitimately empty — an empty children array breaks
// the editor on open — so blank markdown deserializes to one empty paragraph,
// the same default `rich-editor.tsx` starts from.
const EMPTY_DOCUMENT: Value = [{ type: "p", children: [{ text: "" }] }];

const readStdin = async () => {
  const chunks: Buffer[] = [];
  for await (const chunk of process.stdin) chunks.push(chunk as Buffer);
  return Buffer.concat(chunks).toString("utf8");
};

const parseRequest = (raw: string): ConvertRequest => {
  const request = JSON.parse(raw) as Partial<ConvertRequest>;
  if (request.direction !== "toMarkdown" && request.direction !== "toBody") {
    throw new Error(`unknown direction: ${String(request.direction)}`);
  }
  if (!Array.isArray(request.items)) {
    throw new Error("items must be an array of strings");
  }
  return { direction: request.direction, items: request.items };
};

const convert = ({ direction, items }: ConvertRequest) => {
  const editor = createMarkdownEditor();
  const markdown = editor.getApi(MarkdownPlugin).markdown;

  return items.map((item, index) => {
    try {
      if (direction === "toMarkdown") {
        const value = JSON.parse(item) as Value;
        return markdown.serialize({ value });
      }
      const value = markdown.deserialize(item);
      return JSON.stringify(value.length > 0 ? value : EMPTY_DOCUMENT);
    } catch (error) {
      // Fail the whole batch rather than returning a partially converted one:
      // a caller that silently accepted a dropped item would write a truncated
      // body back to the database.
      throw new Error(
        `item ${index}: ${error instanceof Error ? error.message : String(error)}`,
      );
    }
  });
};

const main = async () => {
  try {
    const results = convert(parseRequest(await readStdin()));
    process.stdout.write(JSON.stringify({ results }));
  } catch (error) {
    process.stdout.write(
      JSON.stringify({
        error: error instanceof Error ? error.message : String(error),
      }),
    );
    process.exitCode = 1;
  }
};

void main();
