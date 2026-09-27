import { Outlet, createRootRouteWithContext } from "@tanstack/react-router";
import type { QueryClient } from "@tanstack/react-query";

export type RouterContext = {
  queryClient: QueryClient;
};

// The root is bare: public pages (home, login, signup, invites) and the
// signed-in app under /app each bring their own layout.
export const Route = createRootRouteWithContext<RouterContext>()({
  component: Outlet,
});
