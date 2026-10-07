import { expect, test, vi } from "vitest";

vi.mock("./api", () => ({ Service: { LogClientError: vi.fn(() => Promise.resolve()) } }));
import { installClientLog } from "./clientlog";

test("reports uncaught errors and unhandled rejections", () => {
  const target = new EventTarget() as unknown as Window;
  const report = vi.fn(() => Promise.resolve());
  installClientLog(target, report);

  const err = new Error("boom");
  const ev = new Event("error") as any;
  ev.error = err;
  ev.message = "boom";
  target.dispatchEvent(ev);
  expect(report).toHaveBeenCalledWith("error", "boom", err.stack);

  const rej = new Event("unhandledrejection") as any;
  rej.reason = "SET_DNS_FAILED: x";
  target.dispatchEvent(rej);
  expect(report).toHaveBeenLastCalledWith("rejection", "SET_DNS_FAILED: x", "");
});

test("a failing reporter never throws", () => {
  const target = new EventTarget() as unknown as Window;
  installClientLog(target, () => {
    throw new Error("no backend");
  });
  const rej = new Event("unhandledrejection") as any;
  rej.reason = undefined;
  expect(() => target.dispatchEvent(rej)).not.toThrow();
});
