import { Events } from "@wailsio/runtime";
import { Service } from "./api";
import { useGhost } from "./store";
import { initI18n } from "../i18n";

/** startBridge subscribes to Go events and loads the initial state. */
export function startBridge(): () => void {
  const s = useGhost.getState();
  const offs = [
    Events.On("state", (ev: any) => useGhost.getState().setSnapshot(ev.data)),
    Events.On("stats", (ev: any) => useGhost.getState().pushStats(ev.data)),
    Events.On("log", (ev: any) => useGhost.getState().pushLog(ev.data)),
    Events.On("query", (ev: any) => useGhost.getState().pushQuery(ev.data)),
    Events.On("scan:progress", (ev: any) => useGhost.getState().setScan(ev.data.running ? ev.data : null)),
    Events.On("dpi:autotune", (ev: any) => useGhost.getState().setAutotune(ev.data)),
    Events.On("update", (ev: any) => useGhost.getState().setUpdate(ev.data)),
  ];
  void Service.GetSnapshot().then(s.setSnapshot);
  void Service.GetSettings().then((st) => {
    s.setSettings(st);
    void initI18n(st.language === "en" ? "en" : "vi");
  });
  void Service.GetLogs().then((l) => s.setLogs(l ?? []));
  void Service.AppInfo().then(s.setInfo);
  return () => offs.forEach((off) => off());
}
