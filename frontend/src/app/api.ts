// The only module that imports generated Wails bindings. Tests mock it.
export * as Service from "../../bindings/github.com/hashcott/ghostline/internal/app/service";
export type {
  AppError,
  AppInfo,
  AutotuneProgress,
  LogEvent,
  ScanProgress,
  ServerRow,
  Snapshot,
  StatsEvent,
  UpdateInfo,
  LANInfo,
  RulesView,
  RulesCompiled,
  ListsProgress,
  ProxyStatus,
  DeviceInfo,
  FakeSNIView,
  DNSServerStatus,
  FakeSNIStatus,
  SetupCountdown,
} from "../../bindings/github.com/hashcott/ghostline/internal/app/models";
export type { ServeStats } from "../../bindings/github.com/hashcott/ghostline/internal/engine/models";
export type { Cert } from "../../bindings/github.com/hashcott/ghostline/internal/certstore/models";
export type { Rule, Decision, LineError, Source as RuleSource } from "../../bindings/github.com/hashcott/ghostline/internal/rules/models";
export type { List, CatalogItem } from "../../bindings/github.com/hashcott/ghostline/internal/rules/lists/models";
export type { Stats as ProxyStats, ConnEvent } from "../../bindings/github.com/hashcott/ghostline/internal/proxy/models";
export type { Settings, UpstreamProxy } from "../../bindings/github.com/hashcott/ghostline/internal/store/models";
export type { QueryEvent } from "../../bindings/github.com/hashcott/ghostline/internal/engine/models";
export type { Result as ProbeResult } from "../../bindings/github.com/hashcott/ghostline/internal/probe/models";
export type { Adapter } from "../../bindings/github.com/hashcott/ghostline/internal/sysdns/models";

export type StatusName = "disconnected" | "connecting" | "protected" | "degraded" | "disconnecting" | "error";
