import { useTranslation } from "react-i18next";
import { Window } from "@wailsio/runtime";
import { APP_LABEL } from "../../brand";
import type { Mode } from "../../app/store";
import type { Lang } from "../../i18n";
import css from "./TitleBar.module.css";

type Props = {
  mode: Mode;
  onMode: (m: Mode) => void;
  lang: Lang;
  onLang: (l: Lang) => void;
};

const noDrag = { "--wails-draggable": "no-drag" } as React.CSSProperties;

export function TitleBar({ mode, onMode, lang, onLang }: Props) {
  const { t } = useTranslation();
  const other: Lang = lang === "vi" ? "en" : "vi";
  return (
    <header className={css.bar}>
      <div className={css.top} data-drag style={{ "--wails-draggable": "drag" } as React.CSSProperties}>
        <span className={css.label}>{APP_LABEL}</span>
        <div className={css.controls}>
          <button style={noDrag} className={css.lang} aria-label={`${t("title.language")}: ${other.toUpperCase()}`} onClick={() => onLang(other)}>
            {lang.toUpperCase()}/{other.toUpperCase()}
          </button>
          <button style={noDrag} aria-label={t("title.minimise")} onClick={() => void Window.Minimise()}>
            —
          </button>
          <button style={noDrag} aria-label={t("title.close")} onClick={() => void Window.Close()}>
            ✕
          </button>
        </div>
      </div>
      <div className={css.tabs} role="tablist">
        {(["simple", "advanced"] as Mode[]).map((m) => (
          <button
            key={m}
            role="tab"
            aria-selected={mode === m}
            className={mode === m ? css.on : undefined}
            onClick={() => onMode(m)}
          >
            {mode === m ? `[${t(`mode.${m}`)}]` : t(`mode.${m}`)}
          </button>
        ))}
      </div>
    </header>
  );
}
