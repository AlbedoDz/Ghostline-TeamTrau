import { useTranslation } from "react-i18next";
import { Service } from "../../app/api";
import { useGhost } from "../../app/store";
import { Browser } from "@wailsio/runtime";
import { isConnected, powerState, serverSummary, useUptime, useUpdate } from "../../app/format";
import { tCode } from "../../i18n";
import { PowerButton } from "../../components/neon/PowerButton";
import { TerminalPanel } from "../../components/neon/TerminalPanel";
import { Banner } from "../../components/neon/Banner";
import { ConnectError } from "../../components/ConnectError";
import { Warnings } from "../../components/Warnings";
import css from "./SimpleView.module.css";

export function SimpleView({ onOpenLogs, onOpenServers = () => {} }: { onOpenLogs: () => void; onOpenServers?: () => void }) {
  const { t } = useTranslation();
  const snap = useGhost((s) => s.snapshot);
  const settings = useGhost((s) => s.settings);
  const latency = useGhost((s) => s.latency);
  const autotune = useGhost((s) => s.autotune);
  const bannerDismissed = useGhost((s) => s.bannerDismissed);
  const dismissBanner = useGhost((s) => s.dismissBanner);
  const uptime = useUptime(snap.since);
  const update = useUpdate();
  const status = String(snap.status);
  const label = `[ ${t(`status.${status}`)} ]`;

  const onPower = () => {
    if (status === "connecting") void Service.CancelConnect();
    else if (isConnected(status)) void Service.Disconnect();
    else if (status !== "disconnecting") void Service.Connect();
  };


  const lastLatency = latency.length ? latency[latency.length - 1] : snap.latencyMs;
  const blocked = snap.blockedSites ?? [];

  let below;
  if (status === "connecting") {
    below = (
      <TerminalPanel
        lines={[1, 2, 3, 4, 5, 6, 7].map((n) => {
          const state = n < snap.step ? "done" : n === snap.step ? "current" : "todo";
          return (
            <span key={n} data-step={state} className={css[state]}>
              <span className={css.mark}>{state === "done" ? "✓ " : state === "current" ? "› " : "  "}</span>
              <span>{t(`step.${n}`)}</span>
            </span>
          );
        })}
      />
    );
  } else if (isConnected(status)) {
    below = (
      <TerminalPanel
        rows={[
          { k: t("simple.server"), v: serverSummary(snap.servers) },
          { k: t("simple.latency"), v: t("common.ms", { value: lastLatency }) },
          { k: t("simple.dpi"), v: snap.dpi.running ? `${snap.dpi.preset} ✓` : t("common.off"), tone: snap.dpi.running ? "ok" : "dim" },
          { k: t("simple.uptime"), v: uptime },
          ...(snap.proxy?.running ? [{ k: t("simple.proxy"), v: snap.proxy.addr }] : []),
        ]}
      />
    );
  } else if (status === "error") {
    below = (
      <TerminalPanel
        rows={[{ k: t("common.details"), v: <button className={css.link} onClick={onOpenLogs}>{t("common.openLogs")}</button> }]}
      />
    );
  } else {
    below = (
      <TerminalPanel
        rows={[
          { k: t("simple.server"), v: t("simple.serverAuto") },
          { k: t("simple.dpi"), v: settings?.dpi.enabled ? t("common.on") : t("common.off"), tone: settings?.dpi.enabled ? "ok" : "dim" },
        ]}
      />
    );
  }


  return (
    <section className={css.view}>
      <Warnings />
      <div className={css.hero}>
        <PowerButton state={powerState(status)} label={label} onClick={onPower} disabled={status === "disconnecting"} />
        <div className={css.status} data-status={status}>
          {label}
          {(status === "connecting" || isConnected(status)) && <span className={css.cursor}>_</span>}
        </div>
        <div className={css.sub}>
          {status === "disconnected" && t("simple.tapToConnect")}
          {status === "connecting" && t("simple.tapToCancel")}
          {isConnected(status) && t("simple.encrypted")}
          {status === "error" && snap.error?.code !== "RESTORE_FAILED" && t("simple.errorUnchanged")}
        </div>
      </div>
      <div className={css.bottom}>
        <ConnectError onOpenServers={onOpenServers} onOpenLogs={onOpenLogs} />
        {isConnected(status) && autotune?.running && (
          <Banner tone="warn">{t("simple.autotuning", { preset: autotune.preset, index: autotune.index, total: autotune.total })}</Banner>
        )}
        {isConnected(status) && autotune && !autotune.running && autotune.error && (
          <Banner tone="err">{tCode(`errors.${autotune.error.code}.message`)}</Banner>
        )}
        {isConnected(status) && blocked.length > 0 && !bannerDismissed && !autotune?.running && (
          <Banner
            tone="warn"
            actions={[
              { label: t("simple.autotune"), onClick: () => void Service.StartAutotune(), primary: true },
              { label: t("common.dismiss"), onClick: () => dismissBanner(true) },
            ]}
          >
            {t("simple.blocked", { count: blocked.length, total: settings?.probeSites?.length ?? blocked.length })}
          </Banner>
        )}
        {below}
        {update && (
          <button className={css.update} onClick={() => void Browser.OpenURL(update.url)}>
            {t("settings.update", { tag: update.tag })}
          </button>
        )}
      </div>
    </section>
  );
}
