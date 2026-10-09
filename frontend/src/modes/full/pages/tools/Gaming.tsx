import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Service, type GameStatusResult, type SDRProbeResult } from "../../../../app/api";
import { describeError } from "../../../../i18n";
import css from "../../full.module.css";
import tc from "./tools.module.css";

export function Gaming() {
  const { t } = useTranslation();
  const [status, setStatus] = useState<GameStatusResult | null>(null);
  const [sdrList, setSdrList] = useState<SDRProbeResult[]>([]);
  const [busy, setBusy] = useState(false);
  const [pingBusy, setPingBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);

  const refreshStatus = () => {
    Service.GetGameStatus()
      .then((res) => {
        if (res) {
          setStatus(res);
          if (res.sdrClusters && res.sdrClusters.length > 0) {
            setSdrList(res.sdrClusters);
          }
        }
      })
      .catch((e: unknown) => setError(describeError(e)));
  };

  useEffect(() => {
    refreshStatus();
  }, []);

  const toggleGameMode = async () => {
    if (!status) return;
    setBusy(true);
    setError(null);
    try {
      const next = !status.gameModeActive;
      await Service.SetGameMode(next);
      setNote(next ? t("gaming.noteModeEnabled") : t("gaming.noteModeDisabled"));
      refreshStatus();
    } catch (e) {
      setError(describeError(e));
    } finally {
      setBusy(false);
    }
  };

  const toggleTweaks = async () => {
    if (!status) return;
    setBusy(true);
    setError(null);
    try {
      const next = !status.windowsOptimized;
      await Service.ApplyGamingNetworkTweaks(next);
      setNote(next ? t("gaming.noteTweaksEnabled") : t("gaming.noteTweaksDisabled"));
      refreshStatus();
    } catch (e) {
      setError(describeError(e));
    } finally {
      setBusy(false);
    }
  };

  const measureSdrPings = async () => {
    setPingBusy(true);
    setError(null);
    try {
      const list = await Service.GetSDRRelayPings();
      if (list) setSdrList(list);
    } catch (e) {
      setError(describeError(e));
    } finally {
      setPingBusy(false);
    }
  };

  return (
    <div className={css.page}>
      {error && <div className={`${css.panel} ${tc.poisoned}`}><div className={tc.verdictTitle}>{error}</div></div>}
      {note && <div className={css.panel}><div className={css.ok}>{note}</div></div>}

      <div className={css.grid2}>
        <div className={css.panel}>
          <div className={css.panelTitle}>
            <span>{t("gaming.statusTitle")}</span>
            <span className={status?.gameModeActive ? css.ok : css.dim}>
              {status?.gameModeActive ? t("gaming.active") : t("gaming.standby")}
            </span>
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 8, marginTop: 8 }}>
            <div>
              <span className={css.dim}>{t("gaming.activeGames")}: </span>
              <span className={status?.activeGames?.length ? css.ok : ""}>
                {status?.activeGames?.length ? status.activeGames.join(", ") : t("gaming.none")}
              </span>
            </div>
            <div>
              <span className={css.dim}>{t("gaming.vacSafe")}: </span>
              <span className={status?.dpiDisarmed || status?.gameModeActive ? css.ok : css.dim}>
                {status?.dpiDisarmed || status?.gameModeActive ? t("gaming.driverPurged") : t("gaming.driverNormal")}
              </span>
            </div>
            <div style={{ marginTop: 6 }}>
              <button className={css.button} onClick={toggleGameMode} disabled={busy}>
                {status?.gameModeActive ? t("gaming.disableMode") : t("gaming.enableMode")}
              </button>
            </div>
          </div>
        </div>

        <div className={css.panel}>
          <div className={css.panelTitle}>
            <span>{t("gaming.tweaksTitle")}</span>
            <span className={status?.windowsOptimized ? css.ok : css.dim}>
              {status?.windowsOptimized ? t("gaming.optimized") : t("gaming.default")}
            </span>
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 8, marginTop: 8 }}>
            <div className={css.dim} style={{ fontSize: 12 }}>
              {t("gaming.tweaksDesc")}
            </div>
            <div style={{ marginTop: 6 }}>
              <button className={css.button} onClick={toggleTweaks} disabled={busy}>
                {status?.windowsOptimized ? t("gaming.revertTweaks") : t("gaming.applyTweaks")}
              </button>
            </div>
          </div>
        </div>
      </div>

      <div className={css.panel}>
        <div className={css.panelTitle}>
          <span>{t("gaming.sdrTitle")}</span>
          <button className={css.button} onClick={measureSdrPings} disabled={pingBusy}>
            {pingBusy ? t("gaming.pinging") : t("gaming.measurePings")}
          </button>
        </div>
        <div style={{ marginTop: 10 }}>
          {sdrList.length === 0 ? (
            <div className={css.dim}>{t("gaming.sdrEmpty")}</div>
          ) : (
            <table className={tc.table}>
              <thead>
                <tr>
                  <th style={{ textAlign: "left", padding: "6px 8px" }}>{t("gaming.cluster")}</th>
                  <th style={{ textAlign: "left", padding: "6px 8px" }}>{t("gaming.location")}</th>
                  <th style={{ textAlign: "left", padding: "6px 8px" }}>{t("gaming.ip")}</th>
                  <th style={{ textAlign: "right", padding: "6px 8px" }}>{t("gaming.latency")}</th>
                  <th style={{ textAlign: "right", padding: "6px 8px" }}>{t("gaming.quality")}</th>
                </tr>
              </thead>
              <tbody>
                {sdrList.map((c) => (
                  <tr key={c.clusterId} style={{ borderTop: "1px solid rgba(255,255,255,0.06)" }}>
                    <td style={{ padding: "6px 8px", fontWeight: 600 }}>{c.clusterId.toUpperCase()}</td>
                    <td style={{ padding: "6px 8px" }}>{c.clusterName}</td>
                    <td style={{ padding: "6px 8px", fontFamily: "monospace", color: "#94a3b8" }}>{c.ip}</td>
                    <td style={{ padding: "6px 8px", textAlign: "right", fontWeight: 600 }}>
                      {c.latencyMs > 0 ? `${c.latencyMs} ms` : "—"}
                    </td>
                    <td
                      style={{
                        padding: "6px 8px",
                        textAlign: "right",
                        color:
                          c.quality === "excellent"
                            ? "#22c55e"
                            : c.quality === "good"
                            ? "#38bdf8"
                            : c.quality === "fair"
                            ? "#eab308"
                            : "#ef4444",
                      }}
                    >
                      {c.quality}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </div>
  );
}
