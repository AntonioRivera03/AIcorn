// Bundles scripts/md-convert.ts and everything it imports (platejs,
// @platejs/markdown, the remark pipeline, and this app's editor plugin kits)
// into one dependency-free Node script. The Go MCP server embeds the result, so
// it must not require node_modules to be present at runtime.
import { build } from "esbuild";

const outfile = "../server/assets/bin/md-convert.cjs";

await build({
  entryPoints: ["scripts/md-convert.ts"],
  outfile,
  bundle: true,
  platform: "node",
  target: "node20",
  format: "cjs",
  jsx: "automatic",
  minify: true,
  // @platejs/math pulls in katex's stylesheet, and the editor kits pull in
  // component styling. Nothing here renders, so stylesheets are dropped rather
  // than given a loader (which would drag the font files in too).
  loader: { ".css": "empty" },
  // Resolves the `@/*` alias the app's imports use.
  tsconfig: "tsconfig.app.json",
});

console.log(`Bundled ${outfile}`);
