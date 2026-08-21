import { BaseEquationPlugin, BaseInlineEquationPlugin } from "@platejs/math";
import {
  BaseTableCellHeaderPlugin,
  BaseTableCellPlugin,
  BaseTablePlugin,
  BaseTableRowPlugin,
} from "@platejs/table";
import { createSlateEditor } from "platejs";

import { BaseBasicBlocksKit } from "@/features/editor/plugins/basic-blocks-base-kit";
import { BaseBasicMarksKit } from "@/features/editor/plugins/basic-marks-base-kit";
import { BaseCalloutKit } from "@/features/editor/plugins/callout-base-kit";
import { BaseCodeBlockKit } from "@/features/editor/plugins/code-block-base-kit";
import { BaseListKit } from "@/features/editor/plugins/list-base-kit";
import { BaseTocKit } from "@/features/editor/plugins/toc-base-kit";
import { BaseToggleKit } from "@/features/editor/plugins/toggle-base-kit";
import { MarkdownKit } from "@/features/editor/plugins/markdown-kit";

/**
 * A headless editor carrying the same node inventory as `rich-editor.tsx`, so
 * markdown conversion sees every type a real task body can contain. It uses
 * `createSlateEditor` rather than `usePlateEditor`, which lets it run outside
 * React and outside the browser — `scripts/md-convert.ts` runs it under Node so
 * the Go MCP server can convert task bodies without reimplementing Plate.
 *
 * Only node-defining plugins belong here. `rich-editor.tsx`'s editing plugins
 * (autoformat, slash, dnd, floating toolbar, trailing block, exit break) define
 * no nodes and have no markdown rules. When a plugin that *does* define a node
 * is added there, add it here too or that node will vanish on conversion.
 */
export const createMarkdownEditor = () =>
  createSlateEditor({
    plugins: [
      ...BaseBasicBlocksKit,
      ...BaseBasicMarksKit,
      ...BaseCalloutKit,
      ...BaseCodeBlockKit,
      ...BaseListKit,
      ...MarkdownKit,
      ...BaseTocKit,
      ...BaseToggleKit,
      BaseEquationPlugin,
      BaseInlineEquationPlugin,
      BaseTablePlugin,
      BaseTableRowPlugin,
      BaseTableCellPlugin,
      BaseTableCellHeaderPlugin,
    ],
  });
