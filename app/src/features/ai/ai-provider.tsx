import { useCallback, useEffect, useState, type ReactNode } from "react";
import { useNavigate } from "@tanstack/react-router";
import { AIContext, type AITask } from "@/features/ai/ai-context";
import { AIPanel } from "@/features/ai/ai-panel";
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { Sparkles, Settings2 } from "lucide-react";

export function AIProvider({ children }: { children: ReactNode }) {
  const [task, setTask] = useState<AITask | null>(null);
  const [currentTask, setCurrentTask] = useState<AITask | null>(null);
  const [commandsOpen, setCommandsOpen] = useState(false);
  const navigate = useNavigate();
  const openAI = useCallback((task: AITask) => {
    setTask(task);
    setCurrentTask(task);
  }, []);
  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => {
      if (e.defaultPrevented || !(e.ctrlKey || e.metaKey)) return;
      if (e.key.toLowerCase() === "k") {
        e.preventDefault();
        setCommandsOpen((value) => !value);
      }
      if (e.shiftKey && e.key.toLowerCase() === "a" && currentTask) {
        e.preventDefault();
        openAI(currentTask);
      }
    };
    window.addEventListener("keydown", handleKey);
    return () => window.removeEventListener("keydown", handleKey);
  }, [currentTask, openAI]);
  return (
    <AIContext.Provider value={{ openAI, setCurrentTask }}>
      {children}
      {task && (
        <AIPanel key={task.id} task={task} onClose={() => setTask(null)} />
      )}
      <CommandDialog
        open={commandsOpen}
        onOpenChange={setCommandsOpen}
        title="Commands"
        description="Task AI and settings"
      >
        <CommandInput placeholder="Search commands…" />
        <CommandList>
          <CommandEmpty>No commands found.</CommandEmpty>
          <CommandGroup heading="AI">
            <CommandItem
              disabled={!currentTask}
              onSelect={() => {
                setCommandsOpen(false);
                if (currentTask) openAI(currentTask);
              }}
            >
              <Sparkles /> Ask AI about {currentTask?.name || "an open task"}
            </CommandItem>
            <CommandItem
              onSelect={() => {
                setCommandsOpen(false);
                void navigate({ to: "/personas" });
              }}
            >
              <Settings2 /> AI settings and agents
            </CommandItem>
          </CommandGroup>
        </CommandList>
      </CommandDialog>
    </AIContext.Provider>
  );
}
