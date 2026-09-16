import { useId, type ComponentProps } from "react";
import { Input } from "@/components/ui/input";
import { PERSONA_MODELS } from "@/types/types";

// Shared field keeps custom agents and the fallback model on the same vocabulary.
export function OpenAIModelInput(props: ComponentProps<typeof Input>) {
  const listId = useId();
  return <>
    <Input {...props} list={listId} placeholder={props.placeholder ?? "gpt-5.6-sol"} autoComplete="off" spellCheck={false} />
    <datalist id={listId}>{PERSONA_MODELS.map((model) => <option key={model} value={model} />)}</datalist>
  </>;
}
