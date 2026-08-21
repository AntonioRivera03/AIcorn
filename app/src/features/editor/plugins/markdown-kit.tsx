import type { MdRules } from "@platejs/markdown";
import type { Descendant, SlateEditor } from "platejs";
import type { Plugin } from "unified";

import {
  MarkdownPlugin,
  convertChildrenDeserialize,
  convertNodesSerialize,
  parseAttributes,
  propsToAttributes,
  remarkMdx,
} from "@platejs/markdown";
import { ElementApi, KEYS, getPluginType } from "platejs";
import remarkEmoji from "remark-emoji";
import remarkGfm from "remark-gfm";
import remarkMath from "remark-math";

// A toggle's children are its summary line, which is inline-only. Markdown
// hands that content back wrapped in a paragraph, so unwrap one level to land
// on the exact shape the editor itself produces.
const toSummaryChildren = (
  editor: SlateEditor,
  children: Descendant[],
): Descendant[] => {
  const paragraphType = getPluginType(editor, KEYS.p);
  const summary = children.flatMap((child) =>
    ElementApi.isElement(child) && child.type === paragraphType
      ? child.children
      : [child],
  );
  return summary.length > 0 ? summary : [{ text: "" }];
};

// @platejs/markdown ships no rule for `toggle`, so toggles were dropped in both
// directions. This mirrors the shape of the library's own `callout` rule: an
// MDX JSX flow element named after the plugin key, which remarkMdx stringifies
// as `<toggle>…</toggle>` and parses straight back into a toggle node.
//
// A toggle's own children are only its summary line — its body is the sibling
// blocks carrying a higher `indent`, which markdown has no representation for.
// Those blocks still round-trip as ordinary paragraphs, just un-nested.
const toggleRules: MdRules = {
  toggle: {
    deserialize: (mdastNode, deco, options) => {
      // `editor` is optional on the options type but always populated by the
      // time a rule runs — @platejs/markdown's own default rules assume it too.
      const editor = options.editor!;
      return {
        ...parseAttributes(mdastNode.attributes),
        children: toSummaryChildren(
          editor,
          convertChildrenDeserialize(mdastNode.children, deco, options),
        ),
        type: getPluginType(editor, KEYS.toggle),
      };
    },
    serialize: (slateNode, options) => {
      // Everything but the structural fields becomes an attribute, so a prop
      // added to the toggle node later survives without touching this rule.
      const props: Record<string, unknown> = { ...slateNode };
      delete props.children;
      delete props.type;
      return {
        attributes: propsToAttributes(props),
        children: convertNodesSerialize(slateNode.children, options),
        name: "toggle",
        type: "mdxJsxFlowElement",
      };
    },
  },
};

// remarkMdx (below) also claims bare `{…}` in prose as a JSX expression, and no
// node here maps to one, so without these rules braces — JSON snippets,
// template syntax — would vanish from anything written through markdown. Keep
// the literal text; serializing escapes it back to `\{…}`, which re-parses to
// the same characters.
const mdxExpression = (mdastNode: { value?: string }) =>
  `{${mdastNode.value ?? ""}}`;

const mdxExpressionRules: MdRules = {
  mdxFlowExpression: {
    deserialize: (mdastNode, _deco, options) => ({
      children: [{ text: mdxExpression(mdastNode) }],
      type: getPluginType(options.editor!, KEYS.p),
    }),
  },
  mdxTextExpression: {
    deserialize: (mdastNode, deco) => ({
      ...deco,
      text: mdxExpression(mdastNode),
    }),
  },
};

export const MarkdownKit = [
  MarkdownPlugin.configure({
    options: {
      // remarkMdx is required, not optional: the default rules for `callout`,
      // `toc`, `kbd` and `highlight` all emit mdxJsx* mdast nodes, which
      // remark-stringify cannot render without the MDX extension. Use the
      // re-export rather than `remark-mdx` directly — it carries the tag
      // @platejs/markdown looks for when a caller asks for `withoutMdx`.
      remarkPlugins: [remarkGfm, remarkEmoji as Plugin, remarkMath, remarkMdx],
      rules: { ...toggleRules, ...mdxExpressionRules },
    },
  }),
];
