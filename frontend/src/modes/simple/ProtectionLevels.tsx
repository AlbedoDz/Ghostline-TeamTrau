import { useEffect, useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Service, type NetworkCheck, type Settings } from "../../app/api";
import { useGhost } from "../../app/store";
import { describeError } from "../../i18n";
import { LEVELS, comboOf, levelOf, presetCombo, withCombo, type Level } from "../../app/protection";
import css from "./SimpleView.module.css";

/**
 * ProtectionLevels picks how much Ghostline does: DNS only, DNS with DPI
 * bypass, everything, or the user's own combination from the Full interface.
 */
type Check = { phase: "running" } | { phase: "done"; result: NetworkCheck } | { phase: "error"; message: string };

export function ProtectionLevels({ onOpenFull, disabled, status }: { onOpenFull: () => void; disabled?: boolean; status: string }) {
  const { t } = useTranslation();
  const settings = useGhost((s) => s.settings);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // The level being applied: shown at once, before the engine is up.
  const [pending, setPending] = useState<Level | null>(null);
  const descId = useId();
  const [check, setCheck] = useState<Check | null>(null);
  const autoRan = useRef(false);
  const offline = status === "disconnected" || status === "error";

  const runCheck = () => {
    setCheck({ phase: "running" });
    Service.CheckNetwork()
      .then((result) => setCheck({ phase: "done", result }))
      .catch((e) => setCheck({ phase: "error", message: describeError(e) }));
  };

  // First run: check the network once, by itself, before anything changes.
  useEffect(() => {
    if (!autoRan.current && settings?.simple?.checked === false && offline) {
      autoRan.current = true;
      runCheck();
    }
  }, [settings?.simple?.checked, offline]);

  if (!settings) return null;
  const current = pending ?? levelOf(settings);
  const now = comboOf(settings);

  const answerCheck = async (apply: boolean) => {
    if (apply && check?.phase === "done") await choose(check.result.recommend as Level);
    await Service.MarkNetworkChecked().catch(() => {});
    const s = useGhost.getState().settings;
    if (s) useGhost.getState().setSettings({ ...s, simple: { ...s.simple, checked: true } } as Settings);
    setCheck(null);
  };

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
            data-suggested={check?.phase === "done" && check.result.recommend === l ? "true" : undefined}
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
      {check && (
        <div className={css.check} aria-live="polite">
          {check.phase === "running" && <div>{t("simple.check.running")}</div>}
          {check.phase === "error" && (
            <>
              <div className={css.levelError}>{check.message}</div>
              <div className={css.checkActions}>
                <button onClick={runCheck}>{t("simple.check.retry")}</button>
                <button onClick={() => void answerCheck(false)}>{t("simple.check.skip")}</button>
              </div>
            </>
          )}
          {check.phase === "done" && (
            <>
              <div>
                {t("simple.check.found", {
                  poisoned: check.result.poisoned, blocked: check.result.blocked, total: check.result.sites?.length ?? 0,
                })}
              </div>
              <div className={css.checkSuggest}>
                {t("simple.check.suggest", { level: t(`simple.level.${check.result.recommend}`) })}
              </div>
              {check.result.isp && <div className={css.checkNote}>{t("simple.check.ispNote", { ip: check.result.isp })}</div>}
              <div className={css.checkActions}>
                <button disabled={busy} onClick={() => void answerCheck(true)}>{t("simple.check.apply")}</button>
                <button disabled={busy} onClick={() => void answerCheck(false)}>{t("simple.check.skip")}</button>
              </div>
            </>
          )}
        </div>
      )}
      {!check && offline && settings.simple?.checked !== false && (
        <button className={css.recheck} onClick={runCheck}>{t("simple.check.again")}</button>
      )}
    </div>
  );
}
