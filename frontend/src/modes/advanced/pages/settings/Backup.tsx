import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Service, type ImportPreview } from "../../../../app/api";
import { useGhost } from "../../../../app/store";
import { describeError, initI18n } from "../../../../i18n";
import css from "../../advanced.module.css";
import tc from "../tools/tools.module.css";

const sections = ["settings", "rules", "customServers", "dpiBlacklist", "dpiAutoHostlist"];

/** Backup exports and imports the user's settings (spec 3 §9, §10.2). */
export function Backup() {
  const { t } = useTranslation();
  const status = useGhost((s) => s.snapshot.status);
  const [exportOpen, setExportOpen] = useState(false);
  const [pick, setPick] = useState<string[]>(sections);
  const [preview, setPreview] = useState<ImportPreview | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState<string | null>(null);

  const canImport = status === "disconnected" || status === "error";

  const doExport = () =>
    void Service.ExportSettings(pick)
      .then(() => { setExportOpen(false); setError(null); })
      .catch((e) => setError(describeError(e)));

  const openImport = async () => {
    setError(null);
    setDone(null);
    try {
      const p = await Service.PreviewImport();
      if (p?.token) setPreview(p);
    } catch (e) {
      setError(describeError(e));
    }
  };

  const toggle = (s: string) => setPick((p) => (p.includes(s) ? p.filter((x) => x !== s) : [...p, s]));

  return (
    <div className={`${css.panel} ${tc.ui}`}>
      <div className={css.panelTitle}>{t("settings.backup.title")}</div>
      <div className={css.dim}>{t("settings.backup.note")}</div>
      <div className={css.row}>
        <button onClick={() => setExportOpen(true)}>{t("settings.backup.export")}</button>
        <button disabled={!canImport} onClick={() => void openImport()}>{t("settings.backup.import")}</button>
      </div>
      {!canImport && <div className={css.dim}>{t("errors.IMPORT_WHILE_CONNECTED.message")}</div>}
      {error && <div className={css.bad}>✕ {error}</div>}
      {done && <div className={css.ok}>{done}</div>}

      {exportOpen && (
        <div className={css.dialog} role="dialog">
          <div className={css.dialogBox}>
            <div className={css.panelTitle}>{t("settings.backup.exportTitle")}</div>
            {sections.map((s) => (
              <label key={s} className={css.setting}>
                <span>{t(`settings.backup.sec.${s}`)}</span>
                <input type="checkbox" aria-label={t(`settings.backup.sec.${s}`)} checked={pick.includes(s)} onChange={() => toggle(s)} />
              </label>
            ))}
            <div className={css.dim}>{t("settings.backup.neverExported")}</div>
            <div className={css.row}>
              <button disabled={pick.length === 0} onClick={doExport}>{t("settings.backup.doExport")}</button>
              <button className={css.dim} onClick={() => setExportOpen(false)}>{t("common.cancel")}</button>
            </div>
          </div>
        </div>
      )}

      {preview && (
        <ImportDialog
          p={preview}
          onClose={(msg) => {
            setPreview(null);
            if (msg) setDone(msg);
          }}
        />
      )}
    </div>
  );
}

function ImportDialog({ p, onClose }: { p: ImportPreview; onClose: (done: string | null) => void }) {
  const { t } = useTranslation();
  const pv = p.preview!;
  const secs = pv.sections ?? [];
  const [chosen, setChosen] = useState<string[]>(secs.filter((s) => !(s.errors ?? []).length).map((s) => s.name));
  const [merge, setMerge] = useState(false);
  const [ack, setAck] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const sni = pv.sniRules ?? [];
  const needsAck = sni.length > 0 && chosen.includes("rules");
  const toggle = (s: string) => setChosen((c) => (c.includes(s) ? c.filter((x) => x !== s) : [...c, s]));

  const apply = async (sniRules: string) => {
    try {
      const order = secs.map((s) => s.name).filter((n) => chosen.includes(n));
      await Service.ApplyImport(p.token, { sections: order, merge, sniRules });
      // Every later save sends the whole settings copy: reload it now, or
      // the next save from any page would undo the import.
      const s = await Service.GetSettings();
      if (s) {
        useGhost.getState().setSettings(s);
        void initI18n(s.language === "en" ? "en" : "vi");
      }
      useGhost.getState().bumpRules();
      onClose(t("settings.backup.imported"));
    } catch (e) {
      const msg = describeError(e);
      setError(/IMPORT_EXPIRED/.test(String(e)) ? `${msg}. ${t("errors.IMPORT_EXPIRED.action")}` : msg);
    }
  };

  return (
    <div className={css.dialog} role="dialog">
      <div className={css.dialogBox}>
        <div className={css.panelTitle}>{t("settings.backup.importTitle")}</div>
        <div className={css.dim}>{t("settings.backup.fileInfo", { version: pv.appVersion, date: String(pv.createdAt).slice(0, 10) })}</div>
        {secs.map((s) => {
          const errs = s.errors ?? [];
          return (
            <div key={s.name}>
              <label className={css.setting}>
                <span>{t(`settings.backup.sec.${s.name}`)} <span className={css.dim}>{t("settings.backup.counts", { new: s.new, replaced: s.replaced })}</span></span>
                <input type="checkbox" aria-label={t(`settings.backup.sec.${s.name}`)} disabled={errs.length > 0}
                  checked={chosen.includes(s.name)} onChange={() => toggle(s.name)} />
              </label>
              {errs.map((e, i) => <div key={i} className={css.bad}>✕ {e}</div>)}
            </div>
          );
        })}
        <label className={css.setting}>
          <span>{t("settings.backup.merge")}</span>
          <input type="checkbox" aria-label={t("settings.backup.merge")} checked={merge} onChange={() => setMerge(!merge)} />
        </label>
        {(pv.warnings ?? []).map((w, i) => (
          <div key={i} className={css.warn}>⚠ {t(`settings.backup.warn.${w.code}.${w.code === "flag_off" ? w.detail.replace(/\./g, "_") : "x"}`, { name: w.detail })}</div>
        ))}
        {needsAck && (
          <div className={css.panel}>
            <div className={css.warn}>{t("settings.backup.sniTitle")}</div>
            {sni.map((r, i) => <div key={i} className={css.code}>{r.pattern}</div>)}
            <label>
              <input type="checkbox" aria-label={t("settings.backup.sniAck")} checked={ack} onChange={() => setAck(!ack)} /> {t("settings.backup.sniAck")}
            </label>
            <button onClick={() => void apply("drop")}>{t("settings.backup.sniDrop")}</button>
          </div>
        )}
        {error && <div className={css.bad}>✕ {error}</div>}
        <div className={css.row}>
          <button disabled={chosen.length === 0 || (needsAck && !ack)} onClick={() => void apply(needsAck ? "accept" : "")}>{t("settings.backup.doImport")}</button>
          <button className={css.dim} onClick={() => onClose(null)}>{t("common.cancel")}</button>
        </div>
      </div>
    </div>
  );
}
