import { useEffect } from "react";
import { Sparkles } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useAI } from "@/features/ai/ai-context";

export function AskAIButton({
  taskId,
  taskName,
}: {
  taskId: number;
  taskName: string;
}) {
  const { openAI, setCurrentTask } = useAI();
  useEffect(() => {
    if (taskId) setCurrentTask({ id: taskId, name: taskName });
    return () => setCurrentTask(null);
  }, [taskId, taskName, setCurrentTask]);
  return (
    <Button
      variant="ghost"
      size="sm"
      disabled={!taskId}
      title="Ask AI (Ctrl/⌘ Shift A)"
      onClick={() => openAI({ id: taskId, name: taskName })}
    >
      <Sparkles className="size-4" /> Ask AI
    </Button>
  );
}
