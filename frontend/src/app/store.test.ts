import { beforeEach, expect, test } from "vitest";
import { useGhost } from "./store";

beforeEach(() => useGhost.getState().reset());

test("pushStats keeps last 60 latency points", () => {
  for (let i = 0; i < 70; i++) useGhost.getState().pushStats({ queries: i, latencyMs: i });
  const s = useGhost.getState();
  expect(s.latency).toHaveLength(60);
  expect(s.latency[0]).toBe(10);
  expect(s.queries).toBe(69);
});

test("pushLog keeps last 1000 lines", () => {
  for (let i = 0; i < 1005; i++) useGhost.getState().pushLog({ time: "", source: "system", code: "X" + i });
  expect(useGhost.getState().logs).toHaveLength(1000);
});

test("pushQuery keeps last 500", () => {
  for (let i = 0; i < 520; i++) useGhost.getState().pushQuery({ Domain: "d" + i } as any);
  expect(useGhost.getState().queries500).toHaveLength(500);
});

test("setSnapshot replaces the snapshot", () => {
  useGhost.getState().setSnapshot({ status: "protected" } as any);
  expect(useGhost.getState().snapshot.status).toBe("protected");
});
