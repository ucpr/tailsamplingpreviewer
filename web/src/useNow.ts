import { useEffect, useState } from "react";

// Ticks every intervalMs so components (e.g. the session duration clock)
// re-render without polling the server.
export function useNow(intervalMs = 1000): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return now;
}
