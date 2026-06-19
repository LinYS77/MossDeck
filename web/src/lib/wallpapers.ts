import { useCallback, useEffect, useState } from "react";
import type { Wallpaper } from "./types";

const STORAGE_KEY = "homepage.wallpaper";
const CUSTOM_KEY = "homepage.wallpaper.custom";
const WALLPAPER_CHANGE = "homepage:wallpaper-change";
const WALLPAPER_CUSTOM_CHANGE = "homepage:wallpaper-custom-change";

function readCustomWallpapers(): Wallpaper[] {
  try {
    const raw = window.localStorage.getItem(CUSTOM_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as Wallpaper[];
    return Array.isArray(parsed)
      ? parsed.filter((w) => w.slug && w.src && w.thumb).map((w) => ({ ...w, custom: true }))
      : [];
  } catch {
    return [];
  }
}

function writeCustomWallpapers(wallpapers: Wallpaper[]) {
  try {
    window.localStorage.setItem(CUSTOM_KEY, JSON.stringify(wallpapers));
  } catch {
    /* ignore quota/private-mode failures */
  }
  window.dispatchEvent(new CustomEvent(WALLPAPER_CUSTOM_CHANGE, { detail: wallpapers }));
}

function readStoredSlug(custom = readCustomWallpapers()): string {
  try {
    const v = window.localStorage.getItem(STORAGE_KEY);
    if (v && custom.some((w) => w.slug === v)) return v;
  } catch {
    /* ignore */
  }
  return custom[0]?.slug ?? "";
}

export function wallpaperBySlug(slug: string, custom = readCustomWallpapers()): Wallpaper | null {
  return custom.find((w) => w.slug === slug) ?? custom[0] ?? null;
}

function loadImage(file: File): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file);
    const image = new Image();
    image.onload = () => {
      URL.revokeObjectURL(url);
      resolve(image);
    };
    image.onerror = () => {
      URL.revokeObjectURL(url);
      reject(new Error("Could not read image."));
    };
    image.src = url;
  });
}

async function imageToDataUrl(file: File, maxSize: number, quality: number): Promise<string> {
  const image = await loadImage(file);
  const scale = Math.min(1, maxSize / Math.max(image.naturalWidth, image.naturalHeight));
  const width = Math.max(1, Math.round(image.naturalWidth * scale));
  const height = Math.max(1, Math.round(image.naturalHeight * scale));
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("Canvas is unavailable.");
  ctx.drawImage(image, 0, 0, width, height);
  return canvas.toDataURL("image/jpeg", quality);
}

export async function createCustomWallpaper(file: File): Promise<Wallpaper> {
  const stamp = Date.now().toString(36);
  const safeName = file.name.replace(/\.[^.]+$/, "").replace(/[^a-z0-9]+/gi, " ").trim() || "Custom wallpaper";
  const [src, thumb] = await Promise.all([
    imageToDataUrl(file, 1600, 0.82),
    imageToDataUrl(file, 420, 0.72),
  ]);
  const wallpaper: Wallpaper = {
    slug: `custom-${stamp}`,
    label: safeName,
    src,
    thumb,
    custom: true,
  };
  const next = [wallpaper, ...readCustomWallpapers()].slice(0, 12);
  writeCustomWallpapers(next);
  return wallpaper;
}

export function deleteCustomWallpaper(slug: string): Wallpaper[] {
  const next = readCustomWallpapers().filter((w) => w.slug !== slug);
  writeCustomWallpapers(next);
  try {
    const selected = window.localStorage.getItem(STORAGE_KEY);
    if (selected === slug) {
      if (next[0]) window.localStorage.setItem(STORAGE_KEY, next[0].slug);
      else window.localStorage.removeItem(STORAGE_KEY);
      window.dispatchEvent(new CustomEvent(WALLPAPER_CHANGE, { detail: next[0]?.slug ?? "" }));
    }
  } catch {
    /* ignore */
  }
  return next;
}

export function useWallpaper(): {
  wallpaper: Wallpaper | null;
  wallpapers: Wallpaper[];
  setSlug: (slug: string) => void;
} {
  const [custom, setCustom] = useState<Wallpaper[]>(() => readCustomWallpapers());
  const [slug, setSlugState] = useState<string>(() => readStoredSlug());
  const wallpaper = wallpaperBySlug(slug, custom);

  const setSlug = useCallback((next: string) => {
    setSlugState(next);
    try {
      if (next) window.localStorage.setItem(STORAGE_KEY, next);
      else window.localStorage.removeItem(STORAGE_KEY);
    } catch {
      /* ignore */
    }
    window.dispatchEvent(new CustomEvent(WALLPAPER_CHANGE, { detail: next }));
  }, []);

  useEffect(() => {
    const onStorage = (e: StorageEvent) => {
      if (e.key === STORAGE_KEY) setSlugState(e.newValue ?? "");
      if (e.key === CUSTOM_KEY) setCustom(readCustomWallpapers());
    };
    const onCustomSlug = (e: Event) => {
      setSlugState(((e as CustomEvent).detail as string) ?? "");
    };
    const onCustomWallpapers = () => setCustom(readCustomWallpapers());
    window.addEventListener("storage", onStorage);
    window.addEventListener(WALLPAPER_CHANGE, onCustomSlug);
    window.addEventListener(WALLPAPER_CUSTOM_CHANGE, onCustomWallpapers);
    return () => {
      window.removeEventListener("storage", onStorage);
      window.removeEventListener(WALLPAPER_CHANGE, onCustomSlug);
      window.removeEventListener(WALLPAPER_CUSTOM_CHANGE, onCustomWallpapers);
    };
  }, []);

  return { wallpaper, wallpapers: custom, setSlug };
}

const ENABLED_KEY = "homepage.wallpaper.enabled";
const ENABLE_CHANGE = "homepage:wallpaper-enable-change";

function readEnabled(): boolean {
  try {
    return window.localStorage.getItem(ENABLED_KEY) === "1";
  } catch {
    return false;
  }
}

export function useWallpaperEnabled(): {
  enabled: boolean;
  setEnabled: (v: boolean) => void;
} {
  const [enabled, setEnabledState] = useState<boolean>(() => readEnabled());

  const setEnabled = useCallback((next: boolean) => {
    setEnabledState(next);
    try {
      window.localStorage.setItem(ENABLED_KEY, next ? "1" : "0");
    } catch {
      /* ignore */
    }
    window.dispatchEvent(new CustomEvent(ENABLE_CHANGE, { detail: next }));
  }, []);

  useEffect(() => {
    const onStorage = (e: StorageEvent) => {
      if (e.key === ENABLED_KEY && e.newValue != null) setEnabledState(e.newValue === "1");
    };
    const onCustom = (e: Event) => setEnabledState((e as CustomEvent).detail as boolean);
    window.addEventListener("storage", onStorage);
    window.addEventListener(ENABLE_CHANGE, onCustom);
    return () => {
      window.removeEventListener("storage", onStorage);
      window.removeEventListener(ENABLE_CHANGE, onCustom);
    };
  }, []);

  return { enabled, setEnabled };
}
