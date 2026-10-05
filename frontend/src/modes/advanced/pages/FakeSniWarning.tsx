import { useLayoutEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import css from "../advanced.module.css";

const PARAGRAPHS = ["p1", "p2", "p3", "p4", "p5", "p6"] as const;

/** FakeSniWarning must be scrolled to the end and confirmed before Fake SNI
 * can be turned on (spec 2B 9.1). */
export function FakeSniWarning({ onAck }: { onAck: () => void }) {
  const { t } = useTranslation();
  const box = useRef<HTMLDivElement>(null);
  const [read, setRead] = useState(false);
  const [ticked, setTicked] = useState(false);

  const check = () => {
    const el = box.current;
    if (el && el.scrollTop + el.clientHeight >= el.scrollHeight - 2) setRead(true);
  };
  // Text shorter than the box: nothing to scroll.
  useLayoutEffect(() => {
    const el = box.current;
    if (el && el.scrollHeight > 0 && el.scrollHeight <= el.clientHeight) setRead(true);
  }, []);

  return (
    <div className={css.panel}>
      <div className={css.panelTitle}>{t("fakesni.warning.title")}</div>
      <div ref={box} data-testid="fakesni-warning" onScroll={check} style={{ maxHeight: 220, overflowY: "auto" }}>
        {PARAGRAPHS.map((p) => (
          <p key={p} className={p === "p4" ? css.warn : undefined}>{t(`fakesni.warning.${p}`)}</p>
        ))}
      </div>
      <label className={css.row}>
        <input type="checkbox" checked={ticked} onChange={(e) => setTicked(e.target.checked)} />
        <span>{t("fakesni.warning.understood")}</span>
      </label>
      <button className={css.ok} disabled={!read || !ticked} onClick={onAck}>
        {t("fakesni.warning.continue")}
      </button>
    </div>
  );
}
