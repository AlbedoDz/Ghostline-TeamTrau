import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Service, type ProbeResult } from "../../../app/api";
import { useGhost } from "../../../app/store";
import { saveSettings } from "../../../app/settings";
import { describeError, tCode } from "../../../i18n";
import { Toggle } from "../../../components/neon/Toggle";
import { Chip } from "../../../components/neon/Chip";
import css from "../advanced.module.css";

// entries counts domains in blacklist text the way Go does: one per line,
// blank lines and # comments skipped.
const entries = (text: string) => text.split("\n").filter((l) => l.trim() && !l.trim().startsWith("#")).length;

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
  // saved: the blacklist file as stored; draft: the editor text. newList is
  // true while a first list is being written: scope switches to blacklist
  // only once it is saved.
  const [saved, setSaved] = useState("");
  const [draft, setDraft] = useState("");
  const [newList, setNewList] = useState(false);
  const [listNote, setListNote] = useState<string | null>(null);
  const [sites, setSites] = useState((settings?.probeSites ?? []).join("\n"));

  const dpi = settings?.dpi;
  useEffect(() => {
    if (!dpi) return;
    Service.PreviewDPIArgs(dpi.preset, dpi.customArgs, dpi.scope)
      .then((a) => setPreview(a ?? []))
      .catch(() => setPreview([]));
  }, [dpi?.preset, dpi?.customArgs, dpi?.scope]);
  useEffect(() => {
    Service.GetDPIBlacklist()
      .then((b) => {
        setSaved(b ?? "");
        setDraft((d) => d || (b ?? "")); // keep what was typed meanwhile
      })
      .catch(() => {});
  }, []);

  if (!settings || !dpi) return null;

  const save = async (patch: Parameters<typeof saveSettings>[0]) => setError(await saveSettings(patch));
  const running = snap.dpi?.running;
  const setEnabled = (on: boolean) => {
    const s = useGhost.getState().settings;
    if (s) useGhost.getState().setSettings({ ...s, dpi: { ...s.dpi, enabled: on } });
  };
  const suggested = () => (settings.probeSites ?? []).map((s) => s + "\n").join("");
  const showList = dpi.scope === "blacklist" || newList;
  const pickScope = (sc: "all" | "blacklist") => {
    setError(null);
    setListNote(null);
    if (sc === "all") {
      setNewList(false);
      if (dpi.scope !== "all") void save((s) => ({ ...s, dpi: { ...s.dpi, scope: "all" } }));
      return;
    }
    if (dpi.scope === "blacklist") return;
    if (entries(saved) > 0) {
      void save((s) => ({ ...s, dpi: { ...s.dpi, scope: "blacklist" } }));
      return;
    }
    if (entries(draft) === 0) setDraft(suggested());
    setNewList(true);
  };
  const saveList = async () => {
    setListNote(null);
    if (entries(draft) === 0) {
      setError(tCode("errors.DPI_BLACKLIST_EMPTY.message"));
      return;
    }
    try {
      await Service.SaveDPIBlacklist(draft);
    } catch (e) {
      setError(describeError(e));
      return;
    }
    setError(null);
    setSaved(draft);
    if (newList) {
      setNewList(false);
      await save((s) => ({ ...s, dpi: { ...s.dpi, scope: "blacklist" } }));
    }
    setListNote(running ? t("dpi.blacklistSavedRestart") : t("dpi.blacklistSaved"));
  };
  const cancelList = () => {
    setError(null);
    setListNote(null);
    setNewList(false);
    setDraft(entries(saved) > 0 ? saved : suggested());
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
      <div className={css.dim}>{t("dpi.legalNote")}</div>

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
            <Chip active={!showList} onClick={() => pickScope("all")}>
              {t("dpi.scopeAll")}
            </Chip>
            <Chip active={showList} onClick={() => pickScope("blacklist")}>
              {entries(saved) > 0 ? `${t("dpi.scopeBlacklist")} (${entries(saved)})` : t("dpi.scopeBlacklist")}
            </Chip>
          </span>
        </div>
        {showList && (
          <div className={css.blacklist}>
            <textarea
              aria-label={t("dpi.blacklistTitle")}
              placeholder={t("dpi.blacklistTitle")}
              title={t("dpi.blacklistTitle")}
              style={{ width: "100%", minHeight: 96 }}
              value={draft}
              onChange={(e) => {
                setDraft(e.target.value);
                setListNote(null);
              }}
              spellCheck={false}
            />
            <div className={css.row}>
              <Chip onClick={() => void saveList()} disabled={!newList && draft === saved}>
                {t("common.save")}
              </Chip>
              <Chip onClick={cancelList} disabled={!newList && draft === saved}>
                {t("common.cancel")}
              </Chip>
              {newList && <span className={css.dim}>{t("dpi.blacklistSuggested")}</span>}
              {listNote && <span className={css.ok}>✓ {listNote}</span>}
            </div>
          </div>
        )}
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
          <div className={css.dim}>{t("dpi.webFragmentNote")}</div>
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
