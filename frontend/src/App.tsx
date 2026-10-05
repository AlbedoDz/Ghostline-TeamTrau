import { useEffect } from "react";
import { startBridge } from "./app/bridge";
import { useGhost, type Mode } from "./app/store";
import { Service } from "./app/api";
import { TitleBar } from "./components/neon/TitleBar";
import { SimpleView } from "./modes/simple/SimpleView";
import { AdvancedView } from "./modes/advanced/AdvancedView";
import { FakeSniBanner } from "./components/FakeSniBanner";
import i18n, { initI18n, type Lang } from "./i18n";
import css from "./App.module.css";

function App() {
  const settings = useGhost((s) => s.settings);
  const setSettings = useGhost((s) => s.setSettings);
  useEffect(() => startBridge(), []);

  const mode: Mode = settings?.mode === "advanced" ? "advanced" : "simple";
  const lang: Lang = (i18n.language as Lang) === "en" ? "en" : "vi";

  const onMode = (m: Mode) => {
    if (settings) setSettings({ ...settings, mode: m });
    void Service.SetMode(m);
  };
  const onLang = async (l: Lang) => {
    await initI18n(l);
    if (settings) {
      const next = { ...settings, language: l };
      setSettings(next);
      void Service.SaveSettings(next);
    }
  };

  return (
    <div className={css.app}>
      <TitleBar mode={mode} onMode={onMode} lang={lang} onLang={onLang} />
      <FakeSniBanner />
      <main className={css.main} data-mode={mode}>
        {mode === "simple" ? (
          <SimpleView
            onOpenLogs={() => {
              useGhost.getState().setPage("logs");
              onMode("advanced");
            }}
            onOpenServers={() => {
              useGhost.getState().setPage("servers");
              onMode("advanced");
            }}
          />
        ) : (
          <AdvancedView />
        )}
      </main>
    </div>
  );
}

export default App;
