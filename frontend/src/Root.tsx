import { useEffect, useState } from "react";
import { on } from "./api";
import { App } from "./App";

/** EVENT_RESUME comes when the window shows again after Positron held events while it was closed. */
export const EVENT_RESUME = "resume";

/**
 * Root draws the app and redraws it from fresh data when the window comes back after being
 * closed: while it is closed Positron sends it no events, so every view reloads what it shows.
 * App keeps the page that was open.
 */
export function Root() {
  const [epoch, setEpoch] = useState(0);
  useEffect(() => on(EVENT_RESUME, () => setEpoch((n) => n + 1)), []);
  return <App key={epoch} />;
}
