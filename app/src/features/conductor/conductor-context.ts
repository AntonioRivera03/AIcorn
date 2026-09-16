import { createContext, useContext } from "react";
import type { useConductor } from "./use-conductor";

export const ConductorContext = createContext<ReturnType<typeof useConductor> | null>(null);
export const useBoardConductor = () => useContext(ConductorContext);
