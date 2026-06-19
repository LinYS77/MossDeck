import { useCallback, useEffect, useState } from "react";

export const QUICK_ACCESS_LIMIT_OPTIONS = [3, 5, 7, 12, 14] as const;
export type QuickAccessLimit = typeof QUICK_ACCESS_LIMIT_OPTIONS[number];

const QUICK_ACCESS_LIMIT_KEY = "homepage.quickAccess.limit";
const QUICK_ACCESS_LIMIT_CHANGE = "homepage:quick-access-limit-change";
const DEFAULT_QUICK_ACCESS_LIMIT: QuickAccessLimit = 5;

function normalizeLimit(value: string | null): QuickAccessLimit {
  const n = Number(value);
  return QUICK_ACCESS_LIMIT_OPTIONS.includes(n as QuickAccessLimit)
    ? n as QuickAccessLimit
    : DEFAULT_QUICK_ACCESS_LIMIT;
}

function readQuickAccessLimit(): QuickAccessLimit {
  try {
    return normalizeLimit(window.localStorage.getItem(QUICK_ACCESS_LIMIT_KEY));
  } catch {
    return DEFAULT_QUICK_ACCESS_LIMIT;
  }
}

export function useQuickAccessLimit(): {
  limit: QuickAccessLimit;
  setLimit: (value: QuickAccessLimit) => void;
  options: readonly QuickAccessLimit[];
} {
  const [limit, setLimitState] = useState<QuickAccessLimit>(() => readQuickAccessLimit());

  const setLimit = useCallback((value: QuickAccessLimit) => {
    setLimitState(value);
    try {
      window.localStorage.setItem(QUICK_ACCESS_LIMIT_KEY, String(value));
    } catch {
      /* ignore */
    }
    window.dispatchEvent(new CustomEvent(QUICK_ACCESS_LIMIT_CHANGE, { detail: value }));
  }, []);

  useEffect(() => {
    const onStorage = (e: StorageEvent) => {
      if (e.key === QUICK_ACCESS_LIMIT_KEY) setLimitState(normalizeLimit(e.newValue));
    };
    const onCustom = (e: Event) => {
      const value = (e as CustomEvent).detail as QuickAccessLimit;
      if (QUICK_ACCESS_LIMIT_OPTIONS.includes(value)) setLimitState(value);
    };
    window.addEventListener("storage", onStorage);
    window.addEventListener(QUICK_ACCESS_LIMIT_CHANGE, onCustom);
    return () => {
      window.removeEventListener("storage", onStorage);
      window.removeEventListener(QUICK_ACCESS_LIMIT_CHANGE, onCustom);
    };
  }, []);

  return { limit, setLimit, options: QUICK_ACCESS_LIMIT_OPTIONS };
}
