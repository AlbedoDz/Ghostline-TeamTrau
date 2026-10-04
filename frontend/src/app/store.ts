import { create } from "zustand";
import type {
  AppInfo,
  AutotuneProgress,
  LogEvent,
  QueryEvent,
  ScanProgress,
  Settings,
  Snapshot,
  StatsEvent,
  UpdateInfo,
} from "./api";

export type Mode = "simple" | "advanced";
export type Page = "overview" | "servers" | "dpi" | "logs" | "settings";

const emptySnapshot = {
  status: "disconnected",
  step: 0,
  warnings: [],
  servers: [],
  since: "",
  latencyMs: 0,
  queries: 0,
  dpi: { enabled: false, running: false, preset: "light" },
  blockedSites: [],
} as unknown as Snapshot;

type State = {
  snapshot: Snapshot;
  settings: Settings | null;
  info: AppInfo | null;
  logs: LogEvent[];
  queries500: QueryEvent[];
  latency: number[];
  queries: number;
  scan: ScanProgress | null;
  autotune: AutotuneProgress | null;
  update: UpdateInfo | null;
  page: Page;
  bannerDismissed: boolean;
  queryLog: boolean;
  setQueryLog: (on: boolean) => void;
  setSnapshot: (s: Snapshot) => void;
  setSettings: (s: Settings) => void;
  setInfo: (i: AppInfo) => void;
  setLogs: (l: LogEvent[]) => void;
  pushLog: (l: LogEvent) => void;
  pushQuery: (q: QueryEvent) => void;
  clearQueries: () => void;
  pushStats: (s: StatsEvent) => void;
  setScan: (s: ScanProgress | null) => void;
  setAutotune: (a: AutotuneProgress | null) => void;
  setUpdate: (u: UpdateInfo | null) => void;
  setPage: (p: Page) => void;
  dismissBanner: (v: boolean) => void;
  reset: () => void;
};

const initial = {
  snapshot: emptySnapshot,
  settings: null,
  info: null,
  logs: [] as LogEvent[],
  queries500: [] as QueryEvent[],
  latency: [] as number[],
  queries: 0,
  scan: null,
  autotune: null,
  update: null,
  page: "overview" as Page,
  bannerDismissed: false,
  queryLog: false,
};

const tail = <T,>(arr: T[], v: T, n: number) => {
  const out = arr.length >= n ? arr.slice(arr.length - n + 1) : arr.slice();
  out.push(v);
  return out;
};

export const useGhost = create<State>((set) => ({
  ...initial,
  setSnapshot: (snapshot) =>
    set((s) => ({
      snapshot,
      // A new connection shows the DPI suggestion again.
      bannerDismissed: snapshot.status === "connecting" ? false : s.bannerDismissed,
      latency: snapshot.status === "disconnected" ? [] : s.latency,
      // DPI can be switched from the tray too; the snapshot wins.
      settings:
        s.settings && typeof snapshot.dpi?.enabled === "boolean" && snapshot.dpi.enabled !== s.settings.dpi.enabled
          ? { ...s.settings, dpi: { ...s.settings.dpi, enabled: snapshot.dpi.enabled } }
          : s.settings,
    })),
  setSettings: (settings) => set({ settings }),
  setInfo: (info) => set({ info }),
  setLogs: (logs) => set({ logs: logs.slice(-1000) }),
  pushLog: (l) => set((s) => ({ logs: tail(s.logs, l, 1000) })),
  pushQuery: (q) => set((s) => ({ queries500: tail(s.queries500, q, 500) })),
  clearQueries: () => set({ queries500: [] }),
  pushStats: (st) => set((s) => ({ latency: tail(s.latency, st.latencyMs, 60), queries: st.queries })),
  setScan: (scan) => set({ scan }),
  setAutotune: (autotune) => set({ autotune }),
  setUpdate: (update) => set({ update }),
  setPage: (page) => set({ page }),
  dismissBanner: (bannerDismissed) => set({ bannerDismissed }),
  setQueryLog: (queryLog) => set({ queryLog }),
  reset: () => set({ ...initial }),
}));
