// PolicyAssistantProvider is the seam between the UI (PolicyAssistant.tsx)
// and whatever actually generates policy suggestions. Today only
// chromeProvider.ts (on-device Chrome Prompt API) implements it. A future
// provider backed by an external LLM (e.g. proxied through a new
// `/api/policy/suggest` backend endpoint) can implement the same interface
// without any UI changes — see registry.ts.
import type { PolicyCfg } from "../types";

export type AttributeSource = "resource" | "span";

export interface SelectedAttribute {
  key: string;
  value: string;
  source: AttributeSource;
}

export interface PolicyGenerationContext {
  description: string;
  attributes: SelectedAttribute[];
}

export interface PolicyGenerationResult {
  policies: PolicyCfg[];
  rationale?: string;
}

export type ProviderAvailability = "available" | "downloadable" | "unavailable";

export interface GenerateOptions {
  onDownloadProgress?: (pct: number) => void;
  signal?: AbortSignal;
}

export interface PolicyAssistantProvider {
  id: string;
  label: string;
  checkAvailability(): Promise<ProviderAvailability>;
  generate(ctx: PolicyGenerationContext, opts?: GenerateOptions): Promise<PolicyGenerationResult>;
}
