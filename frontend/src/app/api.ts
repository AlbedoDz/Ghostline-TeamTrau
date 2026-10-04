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
} from "../../bindings/github.com/hashcott/ghostline/internal/app/models";
export type { Settings } from "../../bindings/github.com/hashcott/ghostline/internal/store/models";
export type { QueryEvent } from "../../bindings/github.com/hashcott/ghostline/internal/engine/models";
export type { Result as ProbeResult } from "../../bindings/github.com/hashcott/ghostline/internal/probe/models";
export type { Adapter } from "../../bindings/github.com/hashcott/ghostline/internal/sysdns/models";

export type StatusName = "disconnected" | "connecting" | "protected" | "degraded" | "disconnecting" | "error";
