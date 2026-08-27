// Single seam the UI goes through to get a PolicyAssistantProvider. Today
// this always returns the on-device Chrome provider; a future external-LLM
// provider (implementing the same PolicyAssistantProvider interface, e.g.
// proxied through a new backend endpoint) plugs in here without any UI
// changes.
import { chromeProvider } from "./chromeProvider";
import type { PolicyAssistantProvider } from "./types";

export function getDefaultProvider(): PolicyAssistantProvider {
  return chromeProvider;
}
