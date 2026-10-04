import { useTranslation } from "react-i18next";
import { useGhost } from "../../../app/store";
import { isConnected, useUptime } from "../../../app/format";
import { Sparkline } from "../../../components/neon/Sparkline";
import css from "../advanced.module.css";

export function Overview() {
  const { t } = useTranslation();
  const snap = useGhost((s) => s.snapshot);
  const latency = useGhost((s) => s.latency);
  const queries = useGhost((s) => s.queries);
  const proxyStats = useGhost((s) => s.proxyStats);
  const uptime = useUptime(snap.since);
  const status = String(snap.status);
  const servers = snap.servers ?? [];

  return (
    <div className={css.page}>
      <div className={css.panel}>
        <div className={css.panelTitle}>
          <span>{t(`status.${status}`)}</span>
          <span className={css.dim}>{isConnected(status) ? uptime : ""}</span>
        </div>
        <div className={css.dim}>{t("overview.route", { servers: servers.length ? servers.join(", ") : "—" })}</div>
      </div>
      <div className={css.panel}>
        <div className={css.panelTitle}>
          <span>{t("overview.latency60")}</span>
          <span className={css.big}>{t("common.ms", { value: latency.length ? latency[latency.length - 1] : snap.latencyMs })}</span>
        </div>
        <Sparkline points={latency} width={600} height={70} />
      </div>
      <div className={css.grid2}>
        <div className={css.panel}>
          <div className={css.panelTitle}>{t("overview.queries")}</div>
          <div className={css.big}>{(queries || snap.queries || 0).toLocaleString()}</div>
        </div>
        <div className={css.panel}>
          <div className={css.panelTitle}>{t("overview.proxy")}</div>
          {snap.proxy?.running ? (
            <>
              <div className={css.ok}>{snap.proxy.addr}</div>
              <div className={css.dim}>
                {t("proxy.stats.open", { count: proxyStats?.open ?? 0 })} · {t("proxy.stats.lan", { count: proxyStats?.lanClients ?? 0 })}
              </div>
            </>
          ) : (
            <span className={css.dim}>{t("common.off")}</span>
          )}
        </div>
        <div className={css.panel}>
          <div className={css.panelTitle}>{t("overview.inUse")}</div>
          {servers.length === 0 ? (
            <span className={css.dim}>—</span>
          ) : (
            servers.map((s, i) => (
              <div key={s + i} className={css.ok}>
                {s}
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
}
