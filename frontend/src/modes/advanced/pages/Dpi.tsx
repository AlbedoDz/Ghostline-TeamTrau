import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Service, type ProbeResult } from "../../../app/api";
import { useGhost } from "../../../app/store";
import { saveSettings } from "../../../app/settings";
import { describeError, tCode } from "../../../i18n";
import { Toggle } from "../../../components/neon/Toggle";
import { Chip } from "../../../components/neon/Chip";
import css from "../advanced.module.css";

const PRESETS = ["light", "medium", "high", "extreme", "mode1", "mode2", "mode3", "mode4", "mode5", "mode6", "custom"];

export function Dpi() {
  const { t } = useTranslation();
  const settings = useGhost((s) => s.settings);
  const snap = useGhost((s) => s.snapshot);
  const autotune = useGhost((s) => s.autotune);
  const [custom, setCustom] = useState(settings?.dpi.customArgs ?? "");
  const [error, setError] = useState<string | null>(null);
  const [preview, setPreview] = useState<string[]>([]);
  const [probe, setProbe] = useState<ProbeResult[]>([]);
  const [blacklist, setBlacklist] = useState<string | null>(null);
  const [sites, setSites] = useState((settings?.probeSites ?? []).join("\n"));

  const dpi = settings?.dpi;
  useEffect(() => {
    if (!dpi) return;
    Service.PreviewDPIArgs(dpi.preset, dpi.customArgs, dpi.scope)
      .then((a) => setPreview(a ?? []))
      .catch(() => setPreview([]));
  }, [dpi?.preset, dpi?.customArgs, dpi?.scope]);

  if (!settings || !dpi) return null;

  const save = async (patch: Parameters<typeof saveSettings>[0]) => setError(await saveSettings(patch));
  const running = snap.dpi?.running;
  const setEnabled = (on: boolean) => {
    const s = useGhost.getState().settings;
    if (s) useGhost.getState().setSettings({ ...s, dpi: { ...s.dpi, enabled: on } });
  };
  const toggleDPI = (on: boolean) => {
    setError(null);
    setEnabled(on);
    Service.SetDPIEnabled(on).catch((e) => {
      setEnabled(!on);
      setError(describeError(e));
    });
  };

  return (
    <div className={css.page}>
      <div className={css.head}>{t("dpi.title")}</div>

      <div className={css.panel}>
        <div className={css.panelTitle}>
          <span>{t("dpi.goodbyedpi")}</span>
          <Toggle label={t("dpi.goodbyedpi")} checked={dpi.enabled} onChange={toggleDPI} />
        </div>
        <div className={css.setting}>
          <span>{t("dpi.preset")}</span>
          <span className={css.row}>
            <select aria-label="preset" value={dpi.preset} onChange={(e) => void save((s) => ({ ...s, dpi: { ...s.dpi, preset: e.target.value } }))}>
              {PRESETS.map((p) => (
                <option key={p} value={p}>
                  {t(`dpi.presets.${p}`)}
                </option>
              ))}
            </select>
            {autotune?.running ? (
              <Chip onClick={() => void Service.CancelAutotune()}>{t("simple.autotuning", { preset: autotune.preset, index: autotune.index, total: autotune.total })}</Chip>
            ) : (
              <Chip onClick={() => void Service.StartAutotune()}>{t("dpi.autotune")}</Chip>
            )}
          </span>
        </div>
        {dpi.preset === "custom" && (
          <div className={css.setting}>
            <span>{t("dpi.customArgs")}</span>
            <input
              aria-label={t("dpi.customArgs")}
              style={{ flex: 1 }}
              value={custom}
              onChange={(e) => setCustom(e.target.value)}
              onBlur={() => void save((s) => ({ ...s, dpi: { ...s.dpi, customArgs: custom } }))}
            />
          </div>
        )}
        <div className={css.setting}>
          <span>{t("dpi.scope")}</span>
          <span className={css.row}>
            {(["all", "blacklist"] as const).map((sc) => (
              <Chip key={sc} active={dpi.scope === sc} onClick={() => void save((s) => ({ ...s, dpi: { ...s.dpi, scope: sc } }))}>
                {sc === "all" ? t("dpi.scopeAll") : t("dpi.scopeBlacklist")}
              </Chip>
            ))}
            <Chip onClick={() => void Service.GetDPIBlacklist().then((b) => setBlacklist(b ?? ""))}>{t("dpi.edit")}</Chip>
          </span>
        </div>
        {error && <div className={css.bad}>{error}</div>}
        {autotune && !autotune.running && autotune.error && <div className={css.bad}>{tCode(`errors.${autotune.error.code}.message`)}</div>}
        <div className={css.code} aria-label={t("dpi.preview")}>
          goodbyedpi.exe {preview.join(" ")}
        </div>
        {running ? (
          <div className={css.ok}>● {t("log.DPI_STARTED", { preset: snap.dpi.preset })}</div>
        ) : dpi.enabled ? (
          <div className={css.dim}>○ {t(snap.status === "protected" || snap.status === "degraded" ? "dpi.starting" : "dpi.waiting")}</div>
        ) : null}
      </div>

      {blacklist !== null && (
        <div className={css.panel}>
          <div className={css.panelTitle}>{t("dpi.blacklistTitle")}</div>
          <textarea style={{ width: "100%", minHeight: 120 }} value={blacklist} onChange={(e) => setBlacklist(e.target.value)} spellCheck={false} />
          <div className={css.row}>
            <Chip onClick={() => void Service.SaveDPIBlacklist(blacklist).then(() => setBlacklist(null))}>{t("common.save")}</Chip>
            <Chip onClick={() => setBlacklist(null)}>{t("common.cancel")}</Chip>
          </div>
        </div>
      )}

      <div className={css.grid2}>
        <div className={css.panel}>
          <div className={css.panelTitle}>
            <span>{t("dpi.fragment")}</span>
            <Toggle label={t("dpi.fragment")} checked={settings.fragmentDns.enabled} onChange={(v) => void save((s) => ({ ...s, fragmentDns: { ...s.fragmentDns, enabled: v } }))} />
          </div>
          <div className={css.setting}>
            <span>{t("dpi.chunks")}</span>
            <input type="number" min={2} max={20} style={{ width: 70 }} value={settings.fragmentDns.chunks}
              onChange={(e) => void save((s) => ({ ...s, fragmentDns: { ...s.fragmentDns, chunks: Number(e.target.value) } }))} />
          </div>
          <div className={css.setting}>
            <span>{t("dpi.delay")}</span>
            <input type="number" min={0} max={200} style={{ width: 70 }} value={settings.fragmentDns.delayMs}
              onChange={(e) => void save((s) => ({ ...s, fragmentDns: { ...s.fragmentDns, delayMs: Number(e.target.value) } }))} />
          </div>
          {dpi.enabled && <div className={css.dim}>{t("dpi.redundant")}</div>}
        </div>

        <div className={css.panel}>
          <div className={css.panelTitle}>
            <span>{t("dpi.probeSites")}</span>
            <Chip onClick={() => void Service.ProbeNow().then((r) => setProbe(r ?? []))}>{t("dpi.probeAgain")}</Chip>
          </div>
          {(settings.probeSites ?? []).map((site) => {
            const r = probe.find((p) => p.site === site);
            return (
              <div key={site} className={css.setting}>
                <span>{site}</span>
                <span className={r ? (r.stage === "ok" ? css.ok : css.bad) : css.dim}>
                  {r ? t(`dpi.stage.${r.stage}`) : "·"}
                </span>
              </div>
            );
          })}
          <textarea
            aria-label={t("dpi.probeSites")}
            style={{ width: "100%", minHeight: 60, marginTop: 6 }}
            value={sites}
            onChange={(e) => setSites(e.target.value)}
            onBlur={() => void save((s) => ({ ...s, probeSites: sites.split(/\s+/).filter(Boolean) }))}
          />
        </div>
      </div>
    </div>
  );
}
