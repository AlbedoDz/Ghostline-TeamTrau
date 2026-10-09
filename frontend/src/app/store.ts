import { create } from "zustand";
import type {
  AppInfo,
  AutotuneProgress,
  AdvScanProgress,
  CFProgress,
  ConnEvent,
  LogEvent,
  ProxyStats,
  QueryEvent,
  ScanProgress,
  ServeStats,
  SetupCountdown,
  Settings,
  Snapshot,
  StatsEvent,
  UpdateInfo,
} from "./api";

export type Mode = "simple" | "full";
export type Page = "overview" | "servers" | "dpi" | "proxy" | "rules" | "dnsserver" | "fakesni" | "tools" | "logs" | "settings";
export type ToolsTab = "logs" | "lookup" | "scanner" | "cfscan" | "stamp" | "gaming";
export type ProxyTab = "proxy" | "fakesni";

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
  reasons: [],
  proxy: { running: false, addr: "", systemProxy: false, shareLan: false },
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
  /** Results of the running (or last) full scan by server ID, as they arrive. */
  scanResults: Record<string, NonNullable<ScanProgress["result"]>>;
  clearScanResults: () => void;
  autotune: AutotuneProgress | null;
  update: UpdateInfo | null;
  page: Page;
  bannerDismissed: boolean;
  queryLog: boolean;
  proxyStats: ProxyStats | null;
  proxyConns: ConnEvent[];
  rulesVersion: number;
  dnsStats: ServeStats | null;
  setup: SetupCountdown | null;
  certsVersion: number;
  toolsTab: ToolsTab;
  setToolsTab: (t: ToolsTab) => void;
  proxyTab: ProxyTab;
  setProxyTab: (t: ProxyTab) => void;
  advScan: AdvScanProgress | null;
  setAdvScan: (p: AdvScanProgress) => void;
  cfScan: CFProgress | null;
  setCfScan: (p: CFProgress) => void;
  setDnsStats: (s: ServeStats) => void;
  setSetup: (s: SetupCountdown) => void;
  bumpCerts: () => void;
  setProxyStats: (s: ProxyStats) => void;
  pushProxyConn: (c: ConnEvent) => void;
  bumpRules: () => void;
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
  scanResults: {} as Record<string, NonNullable<ScanProgress["result"]>>,
  autotune: null,
  update: null,
  page: "overview" as Page,
  bannerDismissed: false,
  queryLog: false,
  proxyStats: null as ProxyStats | null,
  proxyConns: [] as ConnEvent[],
  rulesVersion: 0,
  dnsStats: null as ServeStats | null,
  setup: null as SetupCountdown | null,
  certsVersion: 0,
  toolsTab: "logs" as ToolsTab,
  proxyTab: "proxy" as ProxyTab,
  advScan: null as AdvScanProgress | null,
  cfScan: null as CFProgress | null,
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
  setScan: (scan) =>
    set((st) => {
      const r = scan?.result;
      const fresh = scan && !st.scan ? {} : st.scanResults; // a new scan starts clean
      return { scan, scanResults: r?.serverId ? { ...fresh, [r.serverId]: r } : fresh };
    }),
  clearScanResults: () => set({ scanResults: {} }),
  setAutotune: (autotune) => set({ autotune }),
  setUpdate: (update) => set({ update }),
  // Logs and Fake SNI are tabs of Tools and Proxy: links to them open the
  // host page on that tab.
  setPage: (page) =>
    set(page === "logs" ? { page: "tools", toolsTab: "logs" } : page === "fakesni" ? { page: "proxy", proxyTab: "fakesni" } : { page }),
  setToolsTab: (toolsTab) => set({ toolsTab }),
  setProxyTab: (proxyTab) => set({ proxyTab }),
  setAdvScan: (advScan) => set({ advScan }),
  setCfScan: (cfScan) => set({ cfScan }),
  dismissBanner: (bannerDismissed) => set({ bannerDismissed }),
  setQueryLog: (queryLog) => set(queryLog ? { queryLog } : { queryLog, proxyConns: [] }),
  setProxyStats: (proxyStats) => set({ proxyStats }),
  setDnsStats: (dnsStats) => set({ dnsStats }),
  setSetup: (setup) => set({ setup: setup.url ? setup : null }),
  bumpCerts: () => set((s) => ({ certsVersion: s.certsVersion + 1 })),
  pushProxyConn: (c) => set((s) => ({ proxyConns: tail(s.proxyConns, c, 500) })),
  bumpRules: () => set((s) => ({ rulesVersion: s.rulesVersion + 1 })),
  reset: () => set({ ...initial }),
}));
