import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Service, type Cert, type DeviceInfo } from "../../../app/api";
import { useGhost } from "../../../app/store";
import { describeError, tCode } from "../../../i18n";
import { Toggle } from "../../../components/neon/Toggle";
import { Chip } from "../../../components/neon/Chip";
import { QRCode } from "../../../components/neon/QRCode";
import css from "../full.module.css";

const SETUP_PORT = 8053;

function mmss(sec: number): string {
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

export function DnsServer() {
  const { t } = useTranslation();
  const settings = useGhost((s) => s.settings);
  const snap = useGhost((s) => s.snapshot);
  const stats = useGhost((s) => s.dnsStats);
  const setup = useGhost((s) => s.setup);
  const certsVersion = useGhost((s) => s.certsVersion);
  const [error, setError] = useState<string | null>(null);
  const [port, setPort] = useState(String(settings?.dnsServer?.dohPort ?? 443));
  const [device, setDevice] = useState<DeviceInfo | null>(null);
  const [ssid, setSsid] = useState("");
  const [qr, setQr] = useState<boolean[][] | null>(null);
  const [lanCA, setLanCA] = useState<Cert | null>(null);

  const ds = settings?.dnsServer;
  const server = snap.dnsServer;

  const reloadDevice = () =>
    void Service.GetDeviceInfo().then((d) => {
      setDevice(d);
      setSsid(d?.ssid ?? "");
    });

  useEffect(reloadDevice, [ds?.enabled, ds?.shareLan, ds?.dohPort, server?.running]);

  useEffect(() => {
    void Service.ListCerts().then((cs) => setLanCA((cs ?? []).find((c) => c.subject.startsWith("Ghostline LAN CA")) ?? null));
  }, [certsVersion, server?.running]);

  useEffect(() => {
    if (!setup?.url) {
      setQr(null);
      return;
    }
    void Service.GetQR(setup.url).then((m) => setQr((m ?? []) as boolean[][]));
  }, [setup?.url]);

  if (!settings || !ds) return null;

  const run = (p: Promise<unknown>) => p.then(() => setError(null)).catch((e) => setError(describeError(e)));
  const setServer = (enabled: boolean, shareLan: boolean, dohPort: number) => run(Service.SetDNSServer(enabled, shareLan, dohPort).then(() =>
    useGhost.getState().setSettings({ ...settings, dnsServer: { ...ds, enabled, shareLan, dohPort } })));

  const commitPort = () => {
    const n = Number(port);
    if (!Number.isInteger(n) || n < 1 || n > 65535 || n === 53 || n === SETUP_PORT || n === settings.proxy?.port) {
      setError(t("dnsserver.portInvalid"));
      return;
    }
    if (n !== ds.dohPort) void setServer(ds.enabled, ds.shareLan, n);
  };

  const commitSsid = () => {
    if (ssid === (ds.iosSsid ?? "")) return;
    void run(Service.SetIOSSSID(ssid).then(() => useGhost.getState().setSettings({ ...settings, dnsServer: { ...ds, iosSsid: ssid } })));
  };

  const openSetup = () =>
    void Service.OpenSetupPage()
      .then((url) => useGhost.getState().setSetup({ url, remainingSec: 600 }))
      .catch((e) => setError(describeError(e)));

  const confirmThen = (msg: string, fn: () => Promise<unknown>) => {
    if (window.confirm(msg)) void run(fn());
  };

  const srvErr = server?.error;
  const skipped = Object.entries(server?.skipped ?? {});
  const shared = ds.shareLan && !!server?.running;

  return (
    <div className={css.page}>
      <div className={css.head}>
        <span>{t("dnsserver.title")}</span>
        <span className={css.count}>{server?.running ? t("dnsserver.listening") : t("common.off")}</span>
      </div>

      {srvErr && (
        <div className={css.panel}>
          <div className={css.bad}>✕ {tCode(`errors.${srvErr.code}.message`, flat(srvErr.params))}</div>
          <div className={css.dim}>{tCode(`errors.${srvErr.code}.action`)}</div>
        </div>
      )}

      <div className={css.panel}>
        <div className={css.setting}>
          <span>{t("dnsserver.enabled")}</span>
          <Toggle label={t("dnsserver.enabled")} checked={ds.enabled} onChange={(v) => void setServer(v, ds.shareLan, ds.dohPort)} />
        </div>
        <div className={css.setting}>
          <span>{t("dnsserver.shareLan")}</span>
          <Toggle label={t("dnsserver.shareLan")} checked={ds.shareLan} onChange={(v) => void setServer(ds.enabled, v, ds.dohPort)} />
        </div>
        <div className={css.setting}>
          <span>{t("dnsserver.dohPort")}</span>
          <input type="number" aria-label={t("dnsserver.dohPort")} min={1} max={65535} style={{ width: 90 }} value={port}
            onChange={(e) => setPort(e.target.value)} onBlur={commitPort} />
        </div>
        {error && <div className={css.bad}>{error}</div>}
        <div className={css.dim}>{t("dnsserver.note")}</div>
        {(server?.addrs ?? []).map((a) => (
          <div key={a} className={css.ok}>{a}</div>
        ))}
        {skipped.map(([addr, why]) => (
          <div key={addr} className={css.warn}>⚠ {t("dnsserver.skipped", { addr, why })}</div>
        ))}
        {stats && <div className={css.dim}>{t("dnsserver.stats", { queries: stats.queries, clients: stats.clients10m })}</div>}
      </div>

      <div className={css.panel}>
        <div className={css.panelTitle}>{t("dnsserver.devices.title")}</div>
        {device?.public && <div className={css.warn}>⚠ {t("proxy.lan.public")}</div>}
        {!shared && <div className={css.dim}>{t("dnsserver.devices.notShared")}</div>}
        {(device?.dnsAddrs ?? []).length > 0 && (
          <div className={css.setting}>
            <span>{t("dnsserver.devices.dnsIp")}</span>
            <span>{(device?.dnsAddrs ?? []).map((a) => <div key={a} className={css.big}>{a}</div>)}</span>
          </div>
        )}
        {(device?.dnsAddrs ?? []).length > 0 && <div className={css.dim}>{t("dnsserver.devices.dnsIpHint")}</div>}
        {(device?.dohUrls ?? []).length > 0 && (
          <div className={css.setting}>
            <span>{t("dnsserver.devices.doh")}</span>
            <span>{(device?.dohUrls ?? []).map((u) => <div key={u}>{u}</div>)}</span>
          </div>
        )}
        {device?.fingerprint && (
          <div className={css.setting}>
            <span>{t("dnsserver.devices.fingerprint")}</span>
            <code style={{ wordBreak: "break-all" }}>{device.fingerprint}</code>
          </div>
        )}
        <div className={css.setting}>
          <span>{t("dnsserver.devices.ssid")}</span>
          <input aria-label={t("dnsserver.devices.ssid")} value={ssid} maxLength={32} list="ghostline-wifi-names"
            onChange={(e) => setSsid(e.target.value)} onBlur={commitSsid} />
        </div>
        <datalist id="ghostline-wifi-names">
          {(device?.wifiSuggestions ?? []).map((n) => (
            <option key={n} value={n} />
          ))}
        </datalist>
        {ds.shareLan && !ds.iosSsid && <div className={css.warn}>⚠ {t("dnsserver.devices.ssidWarning")}</div>}
        <div className={css.row} style={{ flexWrap: "wrap" }}>
          {setup ? (
            <Chip onClick={() => void run(Service.CloseSetupPage().then(() => useGhost.getState().setSetup({ url: "", remainingSec: 0 })))}>
              {t("dnsserver.devices.closeSetup")}
            </Chip>
          ) : (
            <Chip disabled={!shared} onClick={openSetup}>{t("dnsserver.devices.openSetup")}</Chip>
          )}
          <Chip onClick={() => void run(Service.SaveDeviceFiles())}>{t("dnsserver.devices.saveFiles")}</Chip>
        </div>
        {!setup && !shared && <div className={css.dim}>{t("dnsserver.devices.setupNeedsShare")}</div>}
        {setup && qr && (
          <div className={css.row} style={{ alignItems: "flex-start", gap: 16, marginTop: 8 }}>
            <QRCode matrix={qr} label={t("dnsserver.devices.setupQr", { url: setup.url })} />
            <div>
              <div className={css.big}>{setup.url}</div>
              <div className={css.dim}>{t("dnsserver.devices.remaining", { time: mmss(setup.remainingSec) })}</div>
            </div>
          </div>
        )}
      </div>

      <div className={css.panel}>
        <div className={css.panelTitle}>{t("dnsserver.ca.title")}</div>
        {lanCA ? (
          <div className={css.dim}>{lanCA.subject} · {String(lanCA.notAfter).slice(0, 10)}</div>
        ) : (
          <div className={css.dim}>{t("dnsserver.ca.none")}</div>
        )}
        <div className={css.row}>
          <Chip onClick={() => confirmThen(t("dnsserver.ca.confirmReset"), () => Service.ResetLANCA().then(reloadDevice))}>{t("dnsserver.ca.reset")}</Chip>
          <Chip onClick={() => confirmThen(t("dnsserver.ca.confirmRemove"), () => Service.RemoveLANCA().then(() => Service.GetSettings()).then((s) => s && useGhost.getState().setSettings(s)))}>
            {t("dnsserver.ca.remove")}
          </Chip>
        </div>
      </div>
    </div>
  );
}

// flat turns array params (addrs) into readable text for messages.
function flat(p: Record<string, unknown> | null | undefined): Record<string, unknown> | undefined {
  if (!p) return undefined;
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(p)) out[k] = Array.isArray(v) ? v.join(", ") : v;
  return out;
}
