import { useId, useState } from "react";
import { useTranslation } from "react-i18next";
import { Service, type Settings } from "../../app/api";
import { useGhost } from "../../app/store";
import { describeError } from "../../i18n";
import { LEVELS, comboOf, levelOf, presetCombo, withCombo, type Level } from "../../app/protection";
import css from "./SimpleView.module.css";

/**
 * ProtectionLevels picks how much Ghostline does: DNS only, DNS with DPI
 * bypass, everything, or the user's own combination from the Full interface.
 * Choosing a level, or the current one again, also scans every server and
 * switches to the fastest (live when connected).
 */
export function ProtectionLevels({ onOpenFull, disabled }: { onOpenFull: () => void; disabled?: boolean }) {
  const { t } = useTranslation();
  const settings = useGhost((s) => s.settings);
  const scan = useGhost((s) => s.scan);
  const tuning = useGhost((s) => !!s.autotune?.running);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // The level being applied: shown at once, before the engine is up.
  const [pending, setPending] = useState<Level | null>(null);
  const descId = useId();
  if (!settings) return null;
  const current = pending ?? levelOf(settings);
  const now = comboOf(settings);

  // A scan already running finds and applies the best servers too.
  const rescan = () => void Service.ScanAll().catch(() => {});

  const choose = async (level: Level) => {
    if (busy || tuning) return;
    if (level === current) return rescan();
    setError(null);
    setBusy(true); // no second click while Go's copy loads
    const stop = () => setBusy(false);
    // Build on Go's copy: auto-tune may have saved a strategy since this
    // one was loaded, and saving the old one would undo it.
    const base = (await Service.GetSettings().catch(() => null)) ?? settings;
    const had = comboOf(base);
    let next: Settings;
    let fakeSniAfter: boolean | null = null;
    if (level === "custom") {
      const c = base.simple?.custom;
      if (!c) {
        stop();
        onOpenFull(); // nothing to bring back: let them set it up
        return;
      }
      next = withCombo(base, c);
      if (c.fakeSni && c.proxy && !had.fakeSni) fakeSniAfter = true;
    } else {
      next = withCombo(base, presetCombo(level));
      if (current === "custom" || (had.fakeSni && !next.proxy?.enabled)) {
        // Remember the user's own setup so "custom" can bring it back.
        next = { ...next, simple: { ...base.simple, custom: had } } as Settings;
      }
    }
    const fakeSniOff = had.fakeSni && !next.proxy?.enabled;
    if (fakeSniOff && !window.confirm(t("simple.level.fakeSniOff"))) return stop();

    setPending(level);
    try {
      if (fakeSniOff) await Service.SetFakeSNI(false); // before the proxy goes away
      // DPI goes through SetDPIEnabled: it starts or stops the engine and
      // marks the snapshot at once (a plain settings save only restarts a
      // running engine, and stale snapshots would flip the level back).
      if (!!next.dpi?.enabled !== had.dpi) await Service.SetDPIEnabled(!!next.dpi?.enabled);
      await Service.SaveSettings(next);
      if (fakeSniAfter) await Service.SetFakeSNI(true); // after the proxy is back
      const fakeSni = { ...base.fakeSni, enabled: fakeSniAfter ?? (fakeSniOff ? false : had.fakeSni) };
      useGhost.getState().setSettings({ ...next, fakeSni } as Settings);
      rescan();
    } catch (e) {
      setError(describeError(e));
      // Part of it may have applied: show what Go really has.
      const fresh = await Service.GetSettings().catch(() => null);
      if (fresh) useGhost.getState().setSettings(fresh);
    } finally {
      setBusy(false);
      setPending(null);
    }
  };

  const summary = [
    now.dpi && t("simple.level.part.dpi"),
    now.proxy && (now.systemProxy ? t("simple.level.part.proxy") : t("simple.level.part.proxyNoSystem")),
    now.fakeSni && "Fake SNI",
  ].filter(Boolean).join(" + ") || t("simple.level.part.none");

  return (
    <div className={css.levels}>
      <div className={css.levelRow} role="radiogroup" aria-label={t("simple.level.label")} aria-describedby={descId}>
        {LEVELS.map((l) => (
          <button
            key={l}
            role="radio"
            aria-checked={current === l}
            className={css.level}
            title={t(`simple.level.hint.${l}`)}
            disabled={disabled || busy || tuning}
            onClick={() => void choose(l)}
          >
            {t(`simple.level.${l}`)}
          </button>
        ))}
      </div>
      {/* What the chosen level does, always in view (hover hints go unseen). */}
      <div id={descId} data-testid="level-description" className={css.levelNote}>
        {tuning ? (
          t("simple.level.tuning")
        ) : pending ? (
          t("simple.level.applying", { level: t(`simple.level.${pending}`) })
        ) : scan && !disabled ? ( // while connecting, step 2 shows it
          t("simple.level.scanning", { done: scan.done, total: scan.total })
        ) : current === "custom" ? (
          <>
            {summary} ·{" "}
            <button className={css.link} onClick={onOpenFull}>{t("simple.level.editInFull")}</button>
          </>
        ) : (
          t(`simple.level.hint.${current}`)
        )}
      </div>
      {error && <div className={css.levelError} role="alert">{error}</div>}
    </div>
  );
}
