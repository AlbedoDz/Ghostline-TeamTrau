import { useTranslation } from "react-i18next";
import { useGhost, type ToolsTab } from "../../../../app/store";
import { TabStrip } from "../../TabStrip";
import { Logs } from "../Logs";
import { Lookup } from "./Lookup";
import { Scanner } from "./Scanner";
import { CfScan } from "./CfScan";
import { Stamp } from "./Stamp";
import css from "../../advanced.module.css";
import tc from "./tools.module.css";

const tabs: ToolsTab[] = ["logs", "lookup", "scanner", "cfscan", "stamp"];

/** Tools is the Advanced-mode diagnostics page: logs first, then the tools (spec 3 §10.1). */
export function Tools() {
  const { t } = useTranslation();
  const tab = useGhost((s) => s.toolsTab);
  const setTab = useGhost((s) => s.setToolsTab);
  return (
    <div className={css.tabbed}>
      <TabStrip tabs={tabs.map((id) => ({ id, label: t(`tools.tab.${id}`) }))} active={tab} onSelect={setTab} />
      {tab === "logs" ? (
        <Logs />
      ) : (
        <div className={`${css.page} ${tc.ui}`}>
          {tab === "lookup" && <Lookup />}
          {tab === "scanner" && <Scanner />}
          {tab === "cfscan" && <CfScan />}
          {tab === "stamp" && <Stamp />}
        </div>
      )}
    </div>
  );
}
