import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import vi from "./vi.json";
import en from "./en.json";

export type Lang = "vi" | "en";

export function initI18n(lang: Lang) {
  if (i18n.isInitialized) return i18n.changeLanguage(lang);
  return i18n.use(initReactI18next).init({
    resources: { vi: { translation: vi }, en: { translation: en } },
    lng: lang,
    fallbackLng: "en",
    interpolation: { escapeValue: false }, // React escapes
    returnNull: false,
  });
}

/**
 * tCode translates a key built from a Go code (errors.<CODE>.message,
 * log.<CODE>, step.<n>). A missing key falls back to the code itself.
 */
export function tCode(key: string, params?: Record<string, unknown>): string {
  if (i18n.exists(key)) return i18n.t(key, params as any) as string;
  const parts = key.split(".");
  return parts.length > 1 && /^[A-Z0-9_]+$/.test(parts[1]) ? parts[1] : parts[parts.length - 1];
}

export default i18n;

/**
 * describeError turns an error from a Go binding ("CODE" or "CODE: cause")
 * into the translated message; anything else is shown as-is.
 */
export function describeError(e: unknown): string {
  const msg = e instanceof Error ? e.message : String(e);
  const m = /^([A-Z][A-Z0-9_]{2,})(?::\s*([\s\S]*))?$/.exec(msg.trim());
  if (m && i18n.exists(`errors.${m[1]}.message`)) {
    const text = i18n.t(`errors.${m[1]}.message`) as string;
    return m[2] ? `${text} (${m[2]})` : text;
  }
  return msg;
}
