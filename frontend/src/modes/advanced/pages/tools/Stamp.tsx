import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Service, type StampCard, type StampFields } from "../../../../app/api";
import { describeError } from "../../../../i18n";
import css from "../../advanced.module.css";
import tc from "./tools.module.css";

const protos = ["doh", "dot", "doq", "dnscrypt", "plain"];

const blank: StampFields = {
  proto: "doh", addr: "", host: "", path: "/dns-query", providerName: "", publicKey: "", hashes: [],
  dnssec: false, noLog: false, noFilter: false, stamp: "", usable: false,
};

/** Stamp decodes and builds sdns:// stamps (spec 3 §8). */
export function Stamp() {
  const { t } = useTranslation();
  const [text, setText] = useState("");
  const [cards, setCards] = useState<StampCard[]>([]);
  const [note, setNote] = useState<string | null>(null);
  const [url, setUrl] = useState("");
  const [ip, setIp] = useState("");
  const [f, setF] = useState<StampFields>(blank);
  const [hashes, setHashes] = useState("");
  const [out, setOut] = useState("");
  const [error, setError] = useState<string | null>(null);

  const decode = async () => setCards((await Service.DecodeStamps(text)) ?? []);

  const add = async (stamp: string) => {
    try {
      const [n] = await Service.AddServers(stamp);
      setNote(t("tools.stamp.added", { count: n }));
    } catch (e) {
      setNote(describeError(e));
    }
  };

  const fromURL = async () => {
    setError(null);
    try {
      const got = await Service.StampFromURL(url.trim(), ip.trim());
      setF({ ...blank, ...got });
      setHashes((got.hashes ?? []).join("\n"));
    } catch (e) {
      setError(describeError(e));
    }
  };

  const build = async () => {
    setError(null);
    setOut("");
    const hs = hashes.split(/[\s,]+/).filter(Boolean);
    try {
      setOut(await Service.EncodeStamp({ ...f, hashes: hs }));
    } catch (e) {
      setError(describeError(e));
    }
  };

  const set = (k: keyof StampFields) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) =>
    setF({ ...f, [k]: e.target.type === "checkbox" ? (e.target as HTMLInputElement).checked : e.target.value });

  const isCrypt = f.proto === "dnscrypt";
  const hasHost = ["doh", "dot", "doq"].includes(f.proto);

  return (
    <div className={tc.columns}>
      <div className={css.panel}>
        <div className={css.panelTitle}>{t("tools.stamp.decodeTitle")}</div>
        <textarea aria-label={t("tools.stamp.input")} rows={4} className={tc.wide} value={text}
          placeholder="sdns://…" onChange={(e) => setText(e.target.value)} />
        <button onClick={() => void decode()} disabled={!text.trim()}>{t("tools.stamp.decode")}</button>
        {note && <div className={css.ok}>{note}</div>}
        {cards.map((c, i) => (
          <div key={i} className={tc.card}>
            {c.error || !c.fields ? (
              <div className={css.bad}>✕ {c.line}: {c.error}</div>
            ) : (
              <>
                <div className={tc.cardHead}>{c.fields.proto.toUpperCase()}</div>
                {(
                  [
                    ["addr", c.fields.addr], ["host", c.fields.host], ["path", c.fields.path],
                    ["providerName", c.fields.providerName], ["publicKey", c.fields.publicKey],
                    ["hashes", (c.fields.hashes ?? []).join(" ")],
                  ] as const
                ).filter(([, v]) => v).map(([k, v]) => (
                  <div key={k} className={tc.field}><span className={css.dim}>{t(`tools.stamp.f.${k}`)}</span> <span>{v}</span></div>
                ))}
                <div className={css.dim}>
                  {[c.fields.dnssec && "DNSSEC", c.fields.noLog && "no-log", c.fields.noFilter && "no-filter"].filter(Boolean).join(" · ")}
                </div>
                {c.fields.usable ? (
                  <button onClick={() => void add(c.fields!.stamp)}>{t("tools.stamp.add")}</button>
                ) : (
                  <div className={css.warn}>{t("tools.stamp.notUsable")}</div>
                )}
              </>
            )}
          </div>
        ))}
      </div>

      <div className={css.panel}>
        <div className={css.panelTitle}>{t("tools.stamp.buildTitle")}</div>
        <div className={css.row}>
          <input aria-label="URL" placeholder="https://dns.example/dns-query" className={tc.grow} value={url} onChange={(e) => setUrl(e.target.value)} />
          <input aria-label={t("tools.stamp.ipOptional")} placeholder="1.2.3.4" value={ip} onChange={(e) => setIp(e.target.value)} />
          <button onClick={() => void fromURL()} disabled={!url.trim()}>{t("tools.stamp.fromUrl")}</button>
        </div>
        <div className={css.setting}>
          <span>{t("tools.stamp.f.proto")}</span>
          <select aria-label={t("tools.stamp.f.proto")} value={f.proto} onChange={set("proto")}>
            {protos.map((p) => <option key={p} value={p}>{p}</option>)}
          </select>
        </div>
        <div className={css.setting}>
          <span>{t("tools.stamp.f.addr")}</span>
          <input aria-label={t("tools.stamp.f.addr")} value={f.addr} onChange={set("addr")} />
        </div>
        {hasHost && (
          <div className={css.setting}>
            <span>{t("tools.stamp.f.host")}</span>
            <input aria-label={t("tools.stamp.f.host")} value={f.host} onChange={set("host")} />
          </div>
        )}
        {f.proto === "doh" && (
          <div className={css.setting}>
            <span>{t("tools.stamp.f.path")}</span>
            <input aria-label={t("tools.stamp.f.path")} value={f.path} onChange={set("path")} />
          </div>
        )}
        {isCrypt && (
          <>
            <div className={css.setting}>
              <span>{t("tools.stamp.f.providerName")}</span>
              <input aria-label={t("tools.stamp.f.providerName")} placeholder="2.dnscrypt-cert.example" value={f.providerName} onChange={set("providerName")} />
            </div>
            <div className={css.setting}>
              <span>{t("tools.stamp.f.publicKey")}</span>
              <input aria-label={t("tools.stamp.f.publicKey")} value={f.publicKey} onChange={set("publicKey")} />
            </div>
          </>
        )}
        {hasHost && (
          <div className={css.setting}>
            <span>{t("tools.stamp.f.hashes")}</span>
            <textarea aria-label={t("tools.stamp.f.hashes")} rows={2} value={hashes} onChange={(e) => setHashes(e.target.value)} />
          </div>
        )}
        <div className={css.row}>
          <label><input type="checkbox" aria-label="DNSSEC" checked={f.dnssec} onChange={set("dnssec")} /> DNSSEC</label>
          <label><input type="checkbox" aria-label="no-log" checked={f.noLog} onChange={set("noLog")} /> no-log</label>
          <label><input type="checkbox" aria-label="no-filter" checked={f.noFilter} onChange={set("noFilter")} /> no-filter</label>
        </div>
        <button onClick={() => void build()}>{t("tools.stamp.build")}</button>
        {error && <div className={css.bad}>✕ {error}</div>}
        {out && (
          <>
            <div className={css.code}>{out}</div>
            <div className={css.row}>
              <button onClick={() => void navigator.clipboard?.writeText(out)}>{t("tools.copy")}</button>
              {f.proto !== "plain" && <button onClick={() => void add(out)}>{t("tools.stamp.add")}</button>}
            </div>
          </>
        )}
      </div>
    </div>
  );
}
