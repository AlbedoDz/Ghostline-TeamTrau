import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Service, type ServerRow } from "../../../app/api";
import { useGhost } from "../../../app/store";
import { Chip } from "../../../components/neon/Chip";
import { DataTable } from "../../../components/neon/DataTable";
import { Toggle } from "../../../components/neon/Toggle";
import { ContextMenu, type MenuItem } from "../../../components/neon/ContextMenu";
import { AddServersDialog } from "../AddServersDialog";
import { prettyServerName, serverDetail } from "../../../app/format";
import css from "../full.module.css";

const PROTOCOLS = ["doh", "dot", "doq", "dnscrypt"];
const TAGS = ["no-filter", "adblock", "family"];
const ms = (ns?: number) => Math.round((ns ?? 0) / 1e6);
// Unchecked rows sort after checked ones, failures after successes.
// matches reports whether every word of query is found (case-insensitive)
// in the server's name, provider, protocol, address, IPs or tags.
function matches(r: ServerRow, query: string): boolean {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean);
  if (words.length === 0) return true;
  const s = r.server;
  // A DNSCrypt stamp is base64: searching inside it matches random letters.
  const address = s.address.startsWith("sdns://") ? "sdns://" : s.address;
  const hay = [prettyServerName(s), s.name, s.provider, String(s.protocol), address, ...(s.ips ?? []), ...(s.tags ?? [])].join(" ").toLowerCase();
  return words.every((w) => hay.includes(w));
}

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
  const [query, setQuery] = useState("");
  const [showPinned, setShowPinned] = useState(false);
  const [pinsChanged, setPinsChanged] = useState(false);
  const [menu, setMenu] = useState<{ row: ServerRow; x: number; y: number } | null>(null);
  const status = useGhost((s) => String(s.snapshot.status));
  const connected = status === "protected" || status === "degraded";
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
        if (showPinned && !r.pinned) return false;
        return matches(r, query);
      }),
    [rows, protocols, tags, onlyOk, query, showPinned],
  );
  const okCount = rows.filter((r) => r.result?.ok).length;

  const pinnedIds = rows.filter((r) => r.pinned).map((r) => r.server.id);
  const afterPinChange = () => {
    if (connected) setPinsChanged(true);
    load();
    // Go may turn "pinned only" off when nothing is left pinned.
    void Service.GetSettings().then((st) => st && useGhost.getState().setSettings(st));
  };
  const pin = async (r: ServerRow) => {
    await Service.SetPinned(r.server.id, !r.pinned);
    afterPinChange();
  };
  const pinMany = async (ids: string[], pinned: boolean) => {
    await Service.SetPinnedMany(ids, pinned);
    afterPinChange();
  };
  const setPinnedOnly = (v: boolean) => {
    if (!settings) return;
    const next = { ...settings, pinnedOnly: v };
    useGhost.getState().setSettings(next);
    void Service.SaveSettings(next).then(() => connected && setPinsChanged(true));
  };
  const reconnect = async () => {
    setPinsChanged(false);
    await Service.Disconnect();
    await Service.Connect();
  };

  const useOnly = async (r: ServerRow) => {
    const others = pinnedIds.filter((id) => id !== r.server.id);
    if (others.length > 0 && !window.confirm(t("servers.menu.useOnlyConfirm", { name: prettyServerName(r.server), count: others.length }))) return;
    await Service.UseOnlyServer(r.server.id);
    load();
    void Service.GetSettings().then((st) => st && useGhost.getState().setSettings(st));
    if (connected) await reconnect();
  };
  const recheck = async (r: ServerRow) => {
    const fresh = await Service.CheckServer(r.server.id);
    if (fresh) setRows((rs) => rs.map((x) => (x.server.id === fresh.server.id ? fresh : x)));
  };
  const menuItems = (r: ServerRow): MenuItem[] => {
    const items: MenuItem[] = [
      { label: r.pinned ? t("servers.menu.unpin") : t("servers.menu.pin"), onSelect: () => void pin(r) },
      { label: t("servers.menu.useOnly"), onSelect: () => void useOnly(r) },
      { label: t("servers.menu.recheck"), onSelect: () => void recheck(r) },
      { label: t("servers.menu.copyAddress"), onSelect: () => void navigator.clipboard?.writeText(r.server.address) },
    ];
    if (r.server.ips?.length) items.push({ label: t("servers.menu.copyIP"), onSelect: () => void navigator.clipboard?.writeText(r.server.ips!.join(", ")) });
    if (r.server.source === "custom") items.push({ label: t("servers.menu.remove"), onSelect: () => void Service.RemoveCustomServer(r.server.id).then(load) });
    return items;
  };

  const onScan = () => (scan?.running ? void Service.CancelScan() : void Service.ScanAll());
  const lastScan = rows.map((r) => r.result?.checkedAt).filter(Boolean).sort().pop();

  return (
    <div className={css.page}>
      <div className={css.head} data-row="head">
        <span>
          {t("servers.title")} · {rows.length} <span className={css.count}>[{t("servers.ok", { count: okCount })}]</span>
        </span>
        <span className={css.tools}>
          <span className={css.searchBox} data-search>
            <input
              type="search"
              aria-label={t("servers.search")}
              placeholder={t("servers.searchHint")}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              className={css.search}
            />
            {query && (
              <button className={css.searchClear} aria-label={t("servers.clearSearch")} onClick={() => setQuery("")}>
                ✕
              </button>
            )}
          </span>
          <Chip onClick={onScan}>
            {scan?.running ? t("servers.scanning", { done: scan.done, total: scan.total }) : t("servers.scanAll")}
          </Chip>
          <Chip onClick={() => setAdding(true)}>{t("servers.add")}</Chip>
        </span>
      </div>
      <div className={css.chips} data-row="filters">
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
        ·
        <span title={pinnedIds.length > 0 && !settings?.pinnedOnly ? t("servers.pinnedPreferred") : undefined}>
          <Chip active={showPinned} onClick={() => setShowPinned(!showPinned)}>{t("servers.pinnedChip", { count: pinnedIds.length })}</Chip>
        </span>
        {showPinned && pinnedIds.length > 0 && <Chip onClick={() => void pinMany(pinnedIds, false)}>{t("servers.unpinAll")}</Chip>}
      </div>
      <div className={css.row} data-row="pin">
        <span title={pinnedIds.length === 0 ? t("servers.pinnedOnlyNeedsPin") : undefined}>
          <Toggle showLabel label={t("servers.pinnedOnly")} checked={!!settings?.pinnedOnly} onChange={setPinnedOnly} disabled={pinnedIds.length === 0} />
        </span>
        {query && (
          <span className={css.searchInfo}>
            <span className={css.count}>{t("servers.matched", { count: visible.length })}</span>
            {visible.length > 0 && (
              <Chip onClick={() => void pinMany(visible.map((r) => r.server.id), true)}>{t("servers.pinAllResults", { count: visible.length })}</Chip>
            )}
          </span>
        )}
      </div>
      {/* What "pass" means, and what it cannot catch. */}
      <div className={css.passRule} data-testid="pass-rule">
        {t("servers.passRule", { domain: settings?.testDomain || "www.google.com" })}{" "}
        <button className={css.linkBtn} onClick={() => useGhost.getState().setPage("settings")}>{t("servers.changeDomain")}</button>
      </div>
      {pinsChanged && connected && (
        <div className={css.row}>
          <span className={css.warn}>{t("servers.pinsChanged")}</span>
          <Chip onClick={() => void reconnect()}>{t("servers.reconnect")}</Chip>
        </div>
      )}
      {query && visible.length === 0 && <div className={css.dim}>{t("servers.noMatch")}</div>}
      <DataTable<ServerRow>
        rows={visible}
        rowKey={(r) => r.server.id}
        highlight={(r) => r.inUse}
        pinTop={(r) => r.pinned}
        onRowMenu={(row, x, y) => setMenu({ row, x, y })}
        onRowDoubleClick={(row) => void pin(row)}
        initialSort={{ key: "latency", dir: "asc" }}
        columns={[
          {
            key: "pin",
            label: "",
            width: "28px",
            align: "center",
            render: (r) => (
              <button className={css.pin} aria-pressed={r.pinned} aria-label={`${t("servers.pin")} ${r.server.name}`} onClick={() => void pin(r)}>
                {r.pinned ? "★" : "☆"}
              </button>
            ),
          },
          {
            key: "name",
            label: t("servers.name"),
            render: (r) => (
              <div className={css.nameCell}>
                <span data-name>{prettyServerName(r.server)}</span>
                <span className={css.detail}>{serverDetail(r.server)}</span>
              </div>
            ),
            sort: (a, b) => prettyServerName(a.server).localeCompare(prettyServerName(b.server)),
          },
          {
            key: "protocol",
            label: t("servers.protocol"),
            width: "96px",
            render: (r) => <span className={css.badge}>{String(r.server.protocol).toUpperCase()}</span>,
          },
          {
            key: "latency",
            label: t("servers.latency"),
            width: "84px",
            align: "right",
            render: (r) => {
              if (!r.result?.ok) return <span className={css.dim}>—</span>;
              const v = ms(r.result.latency);
              return <span className={v < 50 ? css.ok : v < 150 ? css.warn : css.bad}>{v} ms</span>;
            },
            sort: (a, b) => rank(a) - rank(b),
          },
          {
            key: "state",
            label: t("servers.state"),
            width: "150px",
            render: (r) =>
              r.inUse ? (
                <span className={css.ok}>● {t("servers.inUse")}</span>
              ) : !r.result ? (
                <span className={css.dim}>{t("servers.notChecked")}</span>
              ) : r.result.ok ? (
                r.auto ? t("servers.pass") : <span className={css.dim} title={t("servers.notAutoHint")}>{t("servers.pass")} · {t("servers.notAuto")}</span>
              ) : (
                <span className={css.bad}>✕ {r.result.reason}</span>
              ),
          },
          { key: "tags", label: t("servers.tags"), width: "120px", render: (r) => <span className={css.dim}>{(r.server.tags ?? []).join(" ")}</span> },
          {
            key: "rm",
            label: "",
            width: "28px",
            align: "center",
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
      </div>
      {menu && <ContextMenu x={menu.x} y={menu.y} items={menuItems(menu.row)} onClose={() => setMenu(null)} />}
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
