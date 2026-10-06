import { useTranslation } from "react-i18next";
import { Service } from "../../app/api";
import { confirmDisconnect } from "../../app/disconnect";
import { useGhost, type Page } from "../../app/store";
import { isConnected } from "../../app/format";
import { Sidebar } from "../../components/neon/Sidebar";
import { Warnings } from "../../components/Warnings";
import { ConnectError } from "../../components/ConnectError";
import { Overview } from "./pages/Overview";
import { Servers } from "./pages/Servers";
import { Dpi } from "./pages/Dpi";
import { Proxy } from "./pages/Proxy";
import { Rules } from "./pages/Rules";
import { DnsServer } from "./pages/DnsServer";
import { FakeSni } from "./pages/FakeSni";
import { Tools } from "./pages/tools/Tools";
import { Settings } from "./pages/Settings";
import { TabStrip } from "./TabStrip";
import css from "./full.module.css";

// Grouped so everyday pages come first; Fake SNI and Logs are tabs of
// Proxy and Tools.
const groups: { header: string; pages: Page[] }[] = [
  { header: "basic", pages: ["overview", "servers", "dpi"] },
  { header: "advanced", pages: ["proxy", "rules", "dnsserver"] },
  { header: "diagnose", pages: ["tools"] },
  { header: "", pages: ["settings"] },
];

function ProxyPage() {
  const { t } = useTranslation();
  const tab = useGhost((s) => s.proxyTab);
  const setTab = useGhost((s) => s.setProxyTab);
  return (
    <div className={css.tabbed}>
      <TabStrip tabs={[{ id: "proxy" as const, label: t("nav.proxy") }, { id: "fakesni" as const, label: t("nav.fakesni") }]} active={tab} onSelect={setTab} />
      {tab === "proxy" ? <Proxy /> : <FakeSni />}
    </div>
  );
}

export function FullView() {
  const { t } = useTranslation();
  const page = useGhost((s) => s.page);
  const setPage = useGhost((s) => s.setPage);
  const snap = useGhost((s) => s.snapshot);
  const latency = useGhost((s) => s.latency);
  const status = String(snap.status);
  const label = t(`status.${status}`);

  const onPower = () => {
    if (status === "connecting") void Service.CancelConnect();
    else if (isConnected(status)) { if (confirmDisconnect(t)) void Service.Disconnect(); }
    else if (status !== "disconnecting") void Service.Connect();
  };

  const tone =
    status === "protected" ? css.ok : status === "degraded" ? css.warn : status === "error" ? css.bad : status === "disconnected" ? css.dim : undefined;
  const action = status === "connecting" ? t("side.cancel") : isConnected(status) ? t("side.disconnect") : t("side.connect");
  const footer = (
    <div className={css.sideCard}>
      <span className={`${css.sideState} ${tone ?? ""}`} style={tone ? undefined : { color: "var(--cyan)" }}>
        {label}
      </span>
      {isConnected(status) && (
        <span className={css.dim}>
          {t("side.summary", { latency: latency.length ? latency[latency.length - 1] : snap.latencyMs, count: snap.servers?.length ?? 0 })}
        </span>
      )}
      <button
        className={`${css.sideBtn} ${isConnected(status) || status === "connecting" ? css.bad : css.ok}`}
        disabled={status === "disconnecting"}
        onClick={onPower}
      >
        {action}
      </button>
    </div>
  );

  return (
    <section className={css.view}>
      <Sidebar
        items={groups.flatMap((g) => [
          { id: `h-${g.header || "end"}`, label: g.header ? t(`nav.group.${g.header}`) : "", header: true },
          ...g.pages.map((p) => ({ id: p, label: t(`nav.${p}`) })),
        ])} active={page} onSelect={(id) => setPage(id as Page)} footer={footer} />
      <div className={css.content}>
        <div style={{ padding: "0 14px" }}>
          <ConnectError onOpenServers={() => setPage("servers")} onOpenLogs={() => setPage("logs")} />
        </div>
        <Warnings />
        {page === "overview" && <Overview />}
        {page === "servers" && <Servers />}
        {page === "dpi" && <Dpi />}
        {page === "proxy" && <ProxyPage />}
        {page === "rules" && <Rules />}
        {page === "dnsserver" && <DnsServer />}
        {page === "tools" && <Tools />}
        {page === "settings" && <Settings />}
      </div>
    </section>
  );
}
