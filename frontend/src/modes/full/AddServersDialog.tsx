import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Service } from "../../app/api";
import css from "./full.module.css";

export function AddServersDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const [text, setText] = useState("");
  const [result, setResult] = useState<{ added: number; bad: string[] } | null>(null);

  const submit = async () => {
    const [added, bad] = await Service.AddServers(text);
    setResult({ added, bad: bad ?? [] });
  };

  const onFile = async (f: File | undefined) => {
    if (f) setText((await f.text()).trim());
  };

  return (
    <div className={css.dialog} role="dialog" aria-label={t("servers.addTitle")}>
      <div className={css.dialogBox}>
        <div className={css.panelTitle}>{t("servers.addTitle")}</div>
        <div className={css.dim}>{t("servers.addHint")}</div>
        <textarea value={text} onChange={(e) => setText(e.target.value)} spellCheck={false} />
        <label className={css.row}>
          <span className={css.dim}>{t("servers.importFile")}</span>
          <input type="file" accept=".txt,.md,.json" onChange={(e) => void onFile(e.target.files?.[0])} />
        </label>
        {result && (
          <div>
            <div className={css.ok}>{t("servers.added", { count: result.added })}</div>
            {result.bad.length > 0 && <div className={css.bad}>{t("servers.rejected", { lines: result.bad.join(", ") })}</div>}
          </div>
        )}
        <div className={css.row}>
          <button className={css.danger} style={{ borderColor: "var(--green)", color: "var(--green)" }} onClick={() => void submit()}>
            {t("common.save")}
          </button>
          <button className={css.dim} onClick={onClose}>
            {t("common.close")}
          </button>
        </div>
      </div>
    </div>
  );
}
