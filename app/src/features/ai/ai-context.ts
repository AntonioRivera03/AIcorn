import { createContext, useContext } from "react";
export type AITask = { id: number; name: string };
export const AIContext = createContext<{
  openAI: (task: AITask) => void;
  setCurrentTask: (task: AITask | null) => void;
}>({ openAI: () => {}, setCurrentTask: () => {} });
export const useAI = () => useContext(AIContext);
