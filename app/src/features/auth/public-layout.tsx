import type { ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import { Squirrel } from "lucide-react";
import { Toaster } from "@/components/ui/sonner";

// The chrome for pages outside the signed-in app: login, signup, onboarding,
// and invites.
export function PublicLayout({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-svh flex-col bg-background text-foreground">
      <header className="flex h-14 shrink-0 items-center px-4 sm:px-6">
        <Link to="/" className="flex items-center gap-2 font-semibold">
          <Squirrel className="size-5" />
          Aycorn
        </Link>
      </header>
      <main className="flex flex-1 justify-center px-4 pt-[8vh] pb-16">
        {children}
      </main>
      <Toaster />
    </div>
  );
}
