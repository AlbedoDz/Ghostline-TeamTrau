import "@testing-library/jest-dom/vitest";
import { afterEach, vi } from "vitest";
import { cleanup } from "@testing-library/react";

afterEach(() => cleanup());

// The Wails runtime talks to the Go side; tests never do.
vi.mock("@wailsio/runtime", () => ({
  Events: { On: vi.fn(() => () => {}), Off: vi.fn(), Emit: vi.fn() },
  Call: { ByID: vi.fn() },
  CancellablePromise: Promise,
  Browser: { OpenURL: vi.fn() },
  Window: { Minimise: vi.fn(), Close: vi.fn() },
}));
