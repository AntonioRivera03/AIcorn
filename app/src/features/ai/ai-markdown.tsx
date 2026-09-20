import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";

export function AIMarkdown({ children }: { children: string }) {
  return (
    <div className="min-w-0 break-words text-sm leading-7 text-foreground [&_p]:my-3 [&_h1]:mb-3 [&_h1]:text-xl [&_h1]:font-semibold [&_h2]:mb-2 [&_h2]:mt-5 [&_h2]:text-lg [&_h2]:font-semibold [&_h3]:mt-4 [&_h3]:font-semibold [&_ul]:list-disc [&_ul]:pl-6 [&_ol]:list-decimal [&_ol]:pl-6 [&_li]:my-1 [&_pre]:my-3 [&_pre]:overflow-auto [&_pre]:rounded-lg [&_pre]:bg-muted [&_pre]:p-3 [&_pre]:text-xs [&_code]:rounded [&_code]:bg-muted [&_code]:px-1 [&_blockquote]:border-l-2 [&_blockquote]:border-border [&_blockquote]:pl-4 [&_blockquote]:text-muted-foreground [&_table]:my-3 [&_table]:block [&_table]:overflow-auto [&_td]:border [&_td]:border-border [&_td]:px-3 [&_th]:border [&_th]:border-border [&_th]:px-3 [&_th]:text-left [&_a]:text-primary [&_a]:underline">
      <Markdown
        remarkPlugins={[remarkGfm]}
        components={{
          // Native agents often cite local files. Those paths are not web routes;
          // show them as file references instead of opening a broken app page.
          a: ({ children, href }) => href && /^(https?:|mailto:)/i.test(href) ? (
            <a href={href} target="_blank" rel="noopener noreferrer">
              {children}
            </a>
          ) : <code title={href}>{children}</code>,
        }}
      >
        {children}
      </Markdown>
    </div>
  );
}
