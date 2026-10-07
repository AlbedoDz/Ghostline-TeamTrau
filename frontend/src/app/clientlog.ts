import { Service } from "./api";

type Reporter = (kind: string, message: string, stack: string) => unknown;

/**
 * installClientLog sends uncaught exceptions and unhandled promise rejections
 * (a Service call whose error nobody shows) to the Go log file.
 */
export function installClientLog(target: Window = window, report: Reporter = (k, m, s) => Service.LogClientError(k, m, s)) {
  const send = (kind: string, err: unknown, fallback: string) => {
    const message = err instanceof Error ? err.message : err == null ? fallback : String(err);
    const stack = err instanceof Error ? (err.stack ?? "") : "";
    try {
      void Promise.resolve(report(kind, message, stack)).catch(() => {});
    } catch {
      // Logging must never throw from an error handler.
    }
  };
  target.addEventListener("error", (e) => send("error", (e as ErrorEvent).error, (e as ErrorEvent).message ?? "error"));
  target.addEventListener("unhandledrejection", (e) => send("rejection", (e as PromiseRejectionEvent).reason, "rejected"));
}
