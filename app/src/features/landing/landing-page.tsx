import { Link } from "@tanstack/react-router";
import { Squirrel } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useMeQuery } from "@/features/auth/queries/me-query";

// TODO: Placeholder home page. Replace with the real marketing page.
export function LandingPage() {
  const { data: me } = useMeQuery();

  return (
    <div className="flex min-h-svh flex-col bg-background text-foreground">
      <header className="flex h-14 shrink-0 items-center justify-between px-4 sm:px-6">
        <Link to="/" className="flex items-center gap-2 font-semibold">
          <Squirrel className="size-5" />
          Aycorn
        </Link>
        <nav className="flex items-center gap-2">
          {me ? (
            <Button asChild size="sm">
              <Link to="/app">Open Aycorn</Link>
            </Button>
          ) : (
            <>
              <Button asChild size="sm" variant="ghost">
                <Link to="/login">Log in</Link>
              </Button>
              <Button asChild size="sm">
                <Link to="/signup">Sign up</Link>
              </Button>
            </>
          )}
        </nav>
      </header>

      <main className="flex flex-1 flex-col items-center justify-center gap-6 px-4 pb-24 text-center">
        <h1 className="max-w-2xl text-4xl font-semibold tracking-tight sm:text-5xl">
          Plan the work. Hand it to your agents.
        </h1>
        <p className="max-w-lg text-muted-foreground">
          Aycorn is a project tracker for you and your team, with AI agents that
          pick up tasks across every project in your organization.
        </p>
        <Button asChild size="lg">
          <Link to={me ? "/app" : "/signup"}>{me ? "Open Aycorn" : "Get started"}</Link>
        </Button>
      </main>
    </div>
  );
}
