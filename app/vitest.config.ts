import path from "path";
import { defineConfig } from "vitest/config";

// Standalone from vite.config.ts on purpose: the tests are headless (no DOM, no
// JSX rendering), so none of the app's build plugins — router codegen, the React
// compiler, Tailwind — need to run. Only the `@/` alias matters.
export default defineConfig({
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.ts", "src/**/*.test.tsx"],
    server: {
      deps: {
        // @platejs/math imports katex's stylesheet. Externalized deps are
        // loaded by Node itself, which cannot parse `.css`; inlining routes
        // them through Vite, which can. scripts/build-md-convert.mjs drops the
        // same stylesheets for the same reason.
        inline: [/@platejs\//],
      },
    },
  },
});
