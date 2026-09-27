import { queryOptions, useQuery } from "@tanstack/react-query";
import type { Me } from "@/features/auth/types";

export const meQueryKey = ["me"] as const;

// Resolves to null when signed out rather than throwing, so route guards can
// redirect instead of erroring.
const fetchMe = async (): Promise<Me | null> => {
  const response = await fetch("/api/auth/me");
  if (response.status === 401) return null;
  if (!response.ok) throw new Error((await response.text()).trim());
  return (await response.json()) as Me;
};

export const meQueryOptions = queryOptions({
  queryKey: meQueryKey,
  queryFn: fetchMe,
  staleTime: 60_000,
  retry: false,
});

export function useMeQuery() {
  return useQuery(meQueryOptions);
}
