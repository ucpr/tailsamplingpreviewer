// Minimal ambient types for Chrome's on-device Prompt API
// (developer.chrome.com/docs/ai/prompt-api). Not part of lib.dom.d.ts yet,
// so we declare just the surface chromeProvider.ts uses. `LanguageModel` is
// absent entirely outside Chrome — always feature-detect with
// `typeof LanguageModel !== "undefined"` before touching it.
export {};

declare global {
  interface LanguageModelDownloadProgressEvent extends Event {
    readonly loaded: number;
  }

  interface LanguageModelCreateMonitor extends EventTarget {
    addEventListener(
      type: "downloadprogress",
      listener: (event: LanguageModelDownloadProgressEvent) => void,
    ): void;
  }

  interface LanguageModelPromptOptions {
    responseConstraint?: unknown;
    signal?: AbortSignal;
  }

  interface LanguageModelSession {
    prompt(input: string, options?: LanguageModelPromptOptions): Promise<string>;
    destroy(): void;
  }

  interface LanguageModelCreateOptions {
    signal?: AbortSignal;
    monitor?(m: LanguageModelCreateMonitor): void;
  }

  const LanguageModel:
    | {
        availability(options?: Record<string, unknown>): Promise<string>;
        create(options?: LanguageModelCreateOptions): Promise<LanguageModelSession>;
      }
    | undefined;
}
