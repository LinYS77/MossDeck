import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import {
  messages,
  type Locale,
  type Messages,
  SUPPORTED_LOCALES,
} from "./messages";

export { type Locale, SUPPORTED_LOCALES };

const STORAGE_KEY = "homepage.language";

interface I18nContextValue {
  locale: Locale;
  setLocale: (l: Locale) => void;
  t: (key: string, vars?: Record<string, string | number>) => string;
}

const I18nContext = createContext<I18nContextValue | null>(null);

function detectLocale(): Locale {
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    if (stored && SUPPORTED_LOCALES.includes(stored as Locale)) {
      return stored as Locale;
    }
  } catch { /* ignore */ }
  // Default to zh-CN for Chinese browsers, en otherwise.
  const nav = navigator.language || "";
  if (nav.startsWith("zh")) return "zh-CN";
  return "en";
}

/** Provides locale state and a `t()` translation function to the subtree.
 *  Wrap your app root with this. */
export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(detectLocale);

  const setLocale = useCallback((l: Locale) => {
    setLocaleState(l);
    try { window.localStorage.setItem(STORAGE_KEY, l); } catch { /* */ }
  }, []);

  // Keep multi-tab language changes in sync.
  useEffect(() => {
    const onStorage = (e: StorageEvent) => {
      if (e.key === STORAGE_KEY && e.newValue &&
          SUPPORTED_LOCALES.includes(e.newValue as Locale)) {
        setLocaleState(e.newValue as Locale);
      }
    };
    window.addEventListener("storage", onStorage);
    return () => window.removeEventListener("storage", onStorage);
  }, []);

  const t = useCallback(
    (key: string, vars?: Record<string, string | number>): string => {
      const msgs: Messages = messages[locale];
      let text = msgs[key];
      // Fallback to English if key is missing in current locale.
      if (text === undefined && locale !== "en") {
        text = messages.en[key];
      }
      if (text === undefined) return key;
      // Interpolate {name} placeholders.
      if (vars) {
        for (const [k, v] of Object.entries(vars)) {
          text = text.replaceAll(`{${k}}`, String(v));
        }
      }
      return text;
    },
    [locale],
  );

  const value = useMemo(() => ({ locale, setLocale, t }), [locale, setLocale, t]);

  return (
    <I18nContext.Provider value={value}>
      {children}
    </I18nContext.Provider>
  );
}

/** Access the i18n context. Must be called inside <I18nProvider>. */
export function useI18n(): I18nContextValue {
  const ctx = useContext(I18nContext);
  if (!ctx) throw new Error("useI18n must be used within I18nProvider");
  return ctx;
}
