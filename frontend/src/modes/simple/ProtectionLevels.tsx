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
 */
export function ProtectionLevels({ onOpenFull, disabled }: { onOpenFull: () => void; disabled?: boolean }) {
  const { t } = useTranslation();
  const settings = useGhost((s) => s.settings);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // The level being applied: shown at once, before the engine is up.
  const [pending, setPending] = useState<Level | null>(null);
  const descId = useId();
  if (!settings) return null;
  const current = pending ?? levelOf(settings);
  const now = comboOf(settings);

  const choose = async (level: Level) => {
    if (busy || (level === current && level !== "custom")) return;
    setError(null);
    let next: Settings;
    let fakeSniAfter: boolean | null = null;
    if (level === "custom") {
      const c = settings.simple?.custom;
      if (!c) {
        onOpenFull(); // nothing to bring back: let them set it up
        return;
      }
      if (current === "custom") return;
      next = withCombo(settings, c);
      if (c.fakeSni && c.proxy && !now.fakeSni) fakeSniAfter = true;
    } else {
      next = withCombo(settings, presetCombo(level));
      if (current === "custom" || (now.fakeSni && !next.proxy?.enabled)) {
        // Remember the user's own setup so "custom" can bring it back.
        next = { ...next, simple: { ...settings.simple, custom: now } } as Settings;
      }
    }
    const fakeSniOff = now.fakeSni && !next.proxy?.enabled;
    if (fakeSniOff && !window.confirm(t("simple.level.fakeSniOff"))) return;

    setBusy(true);
    setPending(level);
    try {
      if (fakeSniOff) await Service.SetFakeSNI(false); // before the proxy goes away
      // DPI goes through SetDPIEnabled: it starts or stops the engine and
      // marks the snapshot at once (a plain settings save only restarts a
      // running engine, and stale snapshots would flip the level back).
      if (!!next.dpi?.enabled !== now.dpi) await Service.SetDPIEnabled(!!next.dpi?.enabled);
      await Service.SaveSettings(next);
      if (fakeSniAfter) await Service.SetFakeSNI(true); // after the proxy is back
      const fakeSni = { ...settings.fakeSni, enabled: fakeSniAfter ?? (fakeSniOff ? false : now.fakeSni) };
      useGhost.getState().setSettings({ ...next, fakeSni } as Settings);
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
            disabled={disabled || busy}
            onClick={() => void choose(l)}
          >
            {t(`simple.level.${l}`)}
          </button>
        ))}
      </div>
      {/* What the chosen level does, always in view (hover hints go unseen). */}
      <div id={descId} data-testid="level-description" className={css.levelNote}>
        {pending ? (
          t("simple.level.applying", { level: t(`simple.level.${pending}`) })
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
