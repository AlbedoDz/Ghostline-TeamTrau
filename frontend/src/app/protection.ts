import type { Settings } from "./api";

/** Protection levels of the Simple interface. */
export type Level = "dns" | "dpi" | "max" | "custom";
export type PresetLevel = Exclude<Level, "custom">;

export const LEVELS: Level[] = ["dns", "dpi", "max", "custom"];

/** The switches a level decides. */
export type Combo = { dpi: boolean; proxy: boolean; systemProxy: boolean; fakeSni: boolean };

export function comboOf(s: Settings): Combo {
  return {
    dpi: !!s.dpi?.enabled,
    proxy: !!s.proxy?.enabled,
    systemProxy: !!s.proxy?.systemProxy,
    fakeSni: !!s.fakeSni?.enabled,
  };
}

/**
 * levelOf reads the level from the settings: it is never stored, so what
 * the Full interface sets always shows correctly here. Fake SNI rides on
 * the proxy and does not change the level.
 */
export function levelOf(s: Settings): Level {
  const c = comboOf(s);
  if (!c.proxy) return c.dpi ? "dpi" : "dns";
  return c.dpi && c.systemProxy ? "max" : "custom";
}

/** withCombo returns s with the level switches set to c (Fake SNI is separate). */
export function withCombo(s: Settings, c: Pick<Combo, "dpi" | "proxy" | "systemProxy">): Settings {
  return {
    ...s,
    dpi: { ...s.dpi, enabled: c.dpi },
    proxy: { ...s.proxy, enabled: c.proxy, systemProxy: c.proxy ? c.systemProxy : !!s.proxy?.systemProxy },
  } as Settings;
}

/** presetCombo is what each preset level turns on. */
export function presetCombo(l: PresetLevel): Pick<Combo, "dpi" | "proxy" | "systemProxy"> {
  switch (l) {
    case "dns":
      return { dpi: false, proxy: false, systemProxy: false };
    case "dpi":
      return { dpi: true, proxy: false, systemProxy: false };
    case "max":
      return { dpi: true, proxy: true, systemProxy: true };
  }
}
