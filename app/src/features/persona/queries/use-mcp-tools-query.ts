import { useQuery } from "@tanstack/react-query";

export type McpTool = {
  name: string;
  description: string;
};

export function useMcpToolsQuery() {
  return useQuery<McpTool[]>({
    queryKey: ["mcpTools"],
    queryFn: async () => {
      const response = await fetch("/api/mcp/tools");
      if (!response.ok) throw new Error(await response.text());
      return response.json() as Promise<McpTool[]>;
    },
  });
}
