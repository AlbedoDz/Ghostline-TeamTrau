import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Service, type ServerRow } from "../../../app/api";
import { useGhost } from "../../../app/store";
import { Chip } from "../../../components/neon/Chip";
import { DataTable } from "../../../components/neon/DataTable";
import { Toggle } from "../../../components/neon/Toggle";
import { AddServersDialog } from "../AddServersDialog";
import css from "../advanced.module.css";

const PROTOCOLS = ["doh", "dot", "doq", "dnscrypt"];
const TAGS = ["no-filter", "adblock", "family"];
const ms = (ns?: number) => Math.round((ns ?? 0) / 1e6);
// Unchecked rows sort after checked ones, failures after successes.
const rank = (r: ServerRow) => (!r.result ? 3e12 : r.result.ok ? ms(r.result.latency) : 2e12);

export function Servers() {
  const { t } = useTranslation();
  const scan = useGhost((s) => s.scan);
  const settings = useGhost((s) => s.settings);
  const servers = useGhost((s) => s.snapshot.servers);
  const [rows, setRows] = useState<ServerRow[]>([]);
  const [protocols, setProtocols] = useState<string[]>(PROTOCOLS);
  const [tags, setTags] = useState<string[]>(TAGS);
  const [onlyOk, setOnlyOk] = useState(false);
  const [adding, setAdding] = useState(false);

  const load = useCallback(() => void Service.ListServers().then((r) => setRows(r ?? [])), []);
  useEffect(load, [load, scan === null, servers?.length]);

  const toggle = (list: string[], v: string, set: (l: string[]) => void) =>
    set(list.includes(v) ? list.filter((x) => x !== v) : [...list, v]);

  const visible = useMemo(
    () =>
      rows.filter((r) => {
        if (!protocols.includes(String(r.server.protocol))) return false;
        const rtags = r.server.tags ?? [];
        if (r.server.source !== "custom" && rtags.length > 0 && !rtags.some((x) => tags.includes(x))) return false;
        if (onlyOk && !r.result?.ok) return false;
        return true;
      }),
    [rows, protocols, tags, onlyOk],
  );
  const okCount = rows.filter((r) => r.result?.ok).length;

  const pin = async (r: ServerRow) => {
    await Service.SetPinned(r.server.id, !r.pinned);
    load();
  };

  const onScan = () => (scan?.running ? void Service.CancelScan() : void Service.ScanAll());
  const lastScan = rows.map((r) => r.result?.checkedAt).filter(Boolean).sort().pop();

  return (
    <div className={css.page}>
      <div className={css.head}>
        <span>
          {t("servers.title")} · {rows.length} <span className={css.count}>[{t("servers.ok", { count: okCount })}]</span>
        </span>
        <span className={css.tools}>
          <Chip onClick={onScan}>
            {scan?.running ? t("servers.scanning", { done: scan.done, total: scan.total }) : t("servers.scanAll")}
          </Chip>
          <Chip onClick={() => setAdding(true)}>{t("servers.add")}</Chip>
        </span>
      </div>
      <div className={css.chips}>
        {t("servers.filter")}:
        {PROTOCOLS.map((p) => (
          <Chip key={p} active={protocols.includes(p)} onClick={() => toggle(protocols, p, setProtocols)}>
            {p}
          </Chip>
        ))}
        ·
        {TAGS.map((p) => (
          <Chip key={p} active={tags.includes(p)} onClick={() => toggle(tags, p, setTags)}>
            {p}
          </Chip>
        ))}
        ·
        <Chip active={onlyOk} onClick={() => setOnlyOk(!onlyOk)}>
          {t("servers.onlyOk")}
        </Chip>
      </div>
      <DataTable<ServerRow>
        rows={visible}
        rowKey={(r) => r.server.id}
        highlight={(r) => r.inUse}
        initialSort={{ key: "latency", dir: "asc" }}
        columns={[
          {
            key: "pin",
            label: "",
            render: (r) => (
              <button className={css.pin} aria-pressed={r.pinned} aria-label={`${t("servers.pin")} ${r.server.name}`} onClick={() => void pin(r)}>
                {r.pinned ? "★" : "☆"}
              </button>
            ),
          },
          { key: "name", label: t("servers.name"), render: (r) => r.server.name, sort: (a, b) => a.server.name.localeCompare(b.server.name) },
          { key: "protocol", label: t("servers.protocol"), render: (r) => String(r.server.protocol) },
          {
            key: "latency",
            label: t("servers.latency"),
            render: (r) => (r.result?.ok ? `${ms(r.result.latency)}ms` : "—"),
            sort: (a, b) => rank(a) - rank(b),
          },
          {
            key: "state",
            label: t("servers.state"),
            render: (r) =>
              r.inUse ? (
                <span className={css.ok}>{t("servers.inUse")}</span>
              ) : !r.result ? (
                <span className={css.dim}>{t("servers.notChecked")}</span>
              ) : r.result.ok ? (
                t("servers.pass")
              ) : (
                <span className={css.bad}>{r.result.reason}</span>
              ),
          },
          { key: "tags", label: t("servers.tags"), render: (r) => <span className={css.dim}>{(r.server.tags ?? []).join(" ")}</span> },
          {
            key: "rm",
            label: "",
            render: (r) =>
              r.server.source === "custom" ? (
                <button className={css.dim} aria-label={`${t("servers.remove")} ${r.server.name}`} onClick={() => void Service.RemoveCustomServer(r.server.id).then(load)}>
                  ✕
                </button>
              ) : null,
          },
        ]}
      />
      <div className={css.foot}>
        <span>{lastScan ? t("servers.scannedAt", { time: new Date(lastScan).toLocaleTimeString() }) : ""}</span>
        <Toggle
          showLabel
          label={t("servers.pinnedOnly")}
          checked={!!settings?.pinnedOnly}
          onChange={(v) => {
            if (!settings) return;
            const next = { ...settings, pinnedOnly: v };
            useGhost.getState().setSettings(next);
            void Service.SaveSettings(next);
          }}
        />
      </div>
      {adding && (
        <AddServersDialog
          onClose={() => {
            setAdding(false);
            load();
          }}
        />
      )}
    </div>
  );
}
