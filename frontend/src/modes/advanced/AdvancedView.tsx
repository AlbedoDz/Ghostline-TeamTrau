import { useTranslation } from "react-i18next";
import { Service } from "../../app/api";
import { useGhost, type Page } from "../../app/store";
import { isConnected } from "../../app/format";
import { Sidebar } from "../../components/neon/Sidebar";
import { Warnings } from "../../components/Warnings";
import { Overview } from "./pages/Overview";
import { Servers } from "./pages/Servers";
import { Dpi } from "./pages/Dpi";
import { Logs } from "./pages/Logs";
import { Settings } from "./pages/Settings";
import css from "./advanced.module.css";

const pages: Page[] = ["overview", "servers", "dpi", "logs", "settings"];

export function AdvancedView() {
  const { t } = useTranslation();
  const page = useGhost((s) => s.page);
  const setPage = useGhost((s) => s.setPage);
  const snap = useGhost((s) => s.snapshot);
  const latency = useGhost((s) => s.latency);
  const status = String(snap.status);
  const label = t(`status.${status}`);

  const onPower = () => {
    if (status === "connecting") void Service.CancelConnect();
    else if (isConnected(status)) void Service.Disconnect();
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
      <Sidebar items={pages.map((p) => ({ id: p, label: t(`nav.${p}`) }))} active={page} onSelect={(id) => setPage(id as Page)} footer={footer} />
      <div className={css.content}>
        <Warnings />
        {page === "overview" && <Overview />}
        {page === "servers" && <Servers />}
        {page === "dpi" && <Dpi />}
        {page === "logs" && <Logs />}
        {page === "settings" && <Settings />}
      </div>
    </section>
  );
}
