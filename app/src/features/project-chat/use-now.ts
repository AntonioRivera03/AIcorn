import { useEffect, useState } from "react";

// The current time, ticking every second while active (a "Working for 12s"
// counter); frozen otherwise.
export function useNow(active: boolean) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!active) return;
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [active]);
  return now;
}
