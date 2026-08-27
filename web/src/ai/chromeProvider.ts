// PolicyAssistantProvider backed by Chrome's on-device Prompt API
// (Gemini Nano). See chromeAi.d.ts for the ambient type surface and
// schema.ts for why responseConstraint avoids oneOf/$ref.
import { buildPrompts } from "./promptBuilder";
import { extractRationale, normalizePolicies } from "./normalize";
import { POLICY_GENERATION_SCHEMA } from "./schema";
import type {
  GenerateOptions,
  PolicyAssistantProvider,
  PolicyGenerationContext,
  PolicyGenerationResult,
  ProviderAvailability,
} from "./types";

function isSupported(): boolean {
  return typeof LanguageModel !== "undefined";
}

function stripCodeFence(text: string): string {
  const trimmed = text.trim();
  const fenced = trimmed.match(/^```(?:json)?\s*([\s\S]*?)\s*```$/i);
  return fenced ? fenced[1] : trimmed;
}

function parseJson(text: string): unknown {
  return JSON.parse(stripCodeFence(text));
}

async function createSession(opts: GenerateOptions): Promise<LanguageModelSession> {
  return LanguageModel!.create({
    signal: opts.signal,
    monitor(m) {
      if (!opts.onDownloadProgress) return;
      m.addEventListener("downloadprogress", (e) => opts.onDownloadProgress!(Math.round(e.loaded * 100)));
    },
  });
}

// Tries a schema-constrained generation first (most reliable); if the
// runtime rejects the schema (e.g. an older Chrome without full structured
// output support) or the model still returns malformed JSON, falls back to
// plain prompting with explicit JSON-only instructions and one repair
// attempt.
async function promptForJson(session: LanguageModelSession, prompt: string, signal?: AbortSignal): Promise<unknown> {
  try {
    const text = await session.prompt(prompt, { responseConstraint: POLICY_GENERATION_SCHEMA, signal });
    return parseJson(text);
  } catch {
    // fall through to unconstrained attempt below
  }

  const instructed = `${prompt}\n\nRespond with ONLY a single JSON object matching this JSON Schema, no prose, no markdown code fences:\n${JSON.stringify(POLICY_GENERATION_SCHEMA)}`;
  const text = await session.prompt(instructed, { signal });
  try {
    return parseJson(text);
  } catch (err) {
    const repairPrompt = `Your previous response could not be parsed as JSON (${err instanceof Error ? err.message : String(err)}). Respond again with ONLY valid JSON matching the schema — no prose, no markdown code fences.`;
    const repaired = await session.prompt(repairPrompt, { signal });
    return parseJson(repaired);
  }
}

class ChromePromptProvider implements PolicyAssistantProvider {
  readonly id = "chrome-builtin";
  readonly label = "Chrome built-in AI (on-device)";

  async checkAvailability(): Promise<ProviderAvailability> {
    if (!isSupported()) {
      console.warn(
        "[policy-assistant] `LanguageModel` is not defined on this page — enable both " +
          "chrome://flags/#optimization-guide-on-device-model and chrome://flags/#prompt-api-for-gemini-nano, " +
          "then restart Chrome.",
      );
      return "unavailable";
    }
    try {
      const availability = await LanguageModel!.availability();
      if (availability === "unavailable") {
        console.warn(
          `[policy-assistant] LanguageModel.availability() reported "unavailable" — check the Model Status ` +
            "tab at chrome://on-device-internals for download errors or unmet hardware requirements.",
        );
        return "unavailable";
      }
      if (availability === "available" || availability === "readily-available") return "available";
      return "downloadable";
    } catch (err) {
      console.error("[policy-assistant] LanguageModel.availability() threw:", err);
      return "unavailable";
    }
  }

  async generate(ctx: PolicyGenerationContext, opts: GenerateOptions = {}): Promise<PolicyGenerationResult> {
    if (!isSupported()) {
      throw new Error("Chrome built-in AI (Prompt API) is not available in this browser.");
    }

    const { system, user } = buildPrompts(ctx);
    const session = await createSession(opts);
    try {
      const raw = await promptForJson(session, `${system}\n\n${user}`, opts.signal);
      return { policies: normalizePolicies(raw), rationale: extractRationale(raw) };
    } finally {
      session.destroy();
    }
  }
}

export const chromeProvider: PolicyAssistantProvider = new ChromePromptProvider();
