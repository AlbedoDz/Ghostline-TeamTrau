import { useTranslation } from "react-i18next";
import { useGhost, type ToolsTab } from "../../../../app/store";
import { Lookup } from "./Lookup";
import { Scanner } from "./Scanner";
import { CfScan } from "./CfScan";
import { Stamp } from "./Stamp";
import css from "../../advanced.module.css";
import tc from "./tools.module.css";

const tabs: ToolsTab[] = ["lookup", "scanner", "cfscan", "stamp"];

/** Tools is the Advanced-mode page with the diagnostic tools (spec 3 §10.1). */
export function Tools() {
  const { t } = useTranslation();
  const tab = useGhost((s) => s.toolsTab);
  const setTab = useGhost((s) => s.setToolsTab);
  return (
    <div className={`${css.page} ${tc.ui}`}>
      <div className={css.head}>
        <span>{t("tools.title")}</span>
        <div className={css.tools} role="tablist">
          {tabs.map((id) => (
            <button key={id} role="tab" aria-selected={tab === id} className={css.chipTab} onClick={() => setTab(id)}>
              {t(`tools.tab.${id}`)}
            </button>
          ))}
        </div>
      </div>
      {tab === "lookup" && <Lookup />}
      {tab === "scanner" && <Scanner />}
      {tab === "cfscan" && <CfScan />}
      {tab === "stamp" && <Stamp />}
    </div>
  );
}
