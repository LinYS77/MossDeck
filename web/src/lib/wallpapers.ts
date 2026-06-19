import { useCallback, useEffect, useState } from "react";
import type { Wallpaper } from "./types";

/** Built-in wallpaper registry. Slugs match files in web/public/wallpaper/. */
export const BUILTIN_WALLPAPERS: Wallpaper[] = [
  { slug: "foggy-forest", label: "Foggy Forest", src: "/wallpaper/foggy-forest.jpg", thumb: "/wallpaper/foggy-forest-thumb.jpg" },
  { slug: "golden-hour", label: "Golden Hour", src: "/wallpaper/golden-hour.jpg", thumb: "/wallpaper/golden-hour-thumb.jpg" },
  { slug: "city-in-the-clouds", label: "City in the Clouds", src: "/wallpaper/city-in-the-clouds.jpg", thumb: "/wallpaper/city-in-the-clouds-thumb.jpg" },
  { slug: "summer-sea", label: "Summer Sea", src: "/wallpaper/summer-sea.jpg", thumb: "/wallpaper/summer-sea-thumb.jpg" },
  { slug: "petals-of-the-moon", label: "Petals of the Moon", src: "/wallpaper/petals-of-the-moon.jpg", thumb: "/wallpaper/petals-of-the-moon-thumb.jpg" },
  { slug: "sunset-ocean", label: "Sunset Ocean", src: "/wallpaper/sunset-ocean.jpg", thumb: "/wallpaper/sunset-ocean-thumb.jpg" },
  { slug: "mountain-glow", label: "Mountain Glow", src: "/wallpaper/mountain-glow.jpg", thumb: "/wallpaper/mountain-glow-thumb.jpg" },
  { slug: "spring-garden", label: "Spring Garden", src: "/wallpaper/spring-garden.jpg", thumb: "/wallpaper/spring-garden-thumb.jpg" },
  { slug: "impression-sunrise", label: "Impression, Sunrise", src: "/wallpaper/impression-sunrise.jpg", thumb: "/wallpaper/impression-sunrise-thumb.jpg" },
  { slug: "seine-spring", label: "Spring by the Seine", src: "/wallpaper/seine-spring.jpg", thumb: "/wallpaper/seine-spring-thumb.jpg" },
];

export const DEFAULT_WALLPAPER_SLUG = "foggy-forest";

const STORAGE_KEY = "homepage.wallpaper";
const CUSTOM_KEY = "homepage.wallpaper.custom";
const WALLPAPER_CHANGE = "homepage:wallpaper-change";
const WALLPAPER_CUSTOM_CHANGE = "homepage:wallpaper-custom-change";

export const WALLPAPERS = BUILTIN_WALLPAPERS;

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

function allWallpapers(custom = readCustomWallpapers()): Wallpaper[] {
  return [...custom, ...BUILTIN_WALLPAPERS];
}

function readStoredSlug(): string {
  try {
    const v = window.localStorage.getItem(STORAGE_KEY);
    if (v && allWallpapers().some((w) => w.slug === v)) return v;
  } catch {
    /* ignore (private mode / disabled storage) */
  }
  return DEFAULT_WALLPAPER_SLUG;
}

export function wallpaperBySlug(slug: string, custom = readCustomWallpapers()): Wallpaper {
  return allWallpapers(custom).find((w) => w.slug === slug) ?? BUILTIN_WALLPAPERS[0];
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
  const next = [wallpaper, ...readCustomWallpapers()].slice(0, 8);
  writeCustomWallpapers(next);
  return wallpaper;
}

export function useWallpaper(): {
  wallpaper: Wallpaper;
  wallpapers: Wallpaper[];
  setSlug: (slug: string) => void;
} {
  const [custom, setCustom] = useState<Wallpaper[]>(() => readCustomWallpapers());
  const [slug, setSlugState] = useState<string>(() => readStoredSlug());
  const wallpapers = allWallpapers(custom);

  const setSlug = useCallback((next: string) => {
    setSlugState(next);
    try {
      window.localStorage.setItem(STORAGE_KEY, next);
    } catch {
      /* ignore */
    }
    window.dispatchEvent(new CustomEvent(WALLPAPER_CHANGE, { detail: next }));
  }, []);

  useEffect(() => {
    const onStorage = (e: StorageEvent) => {
      if (e.key === STORAGE_KEY && e.newValue) setSlugState(e.newValue);
      if (e.key === CUSTOM_KEY) setCustom(readCustomWallpapers());
    };
    const onCustomSlug = (e: Event) => {
      const next = (e as CustomEvent).detail as string;
      if (next) setSlugState(next);
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

  return { wallpaper: wallpaperBySlug(slug, custom), wallpapers, setSlug };
}

/* ---- Optional decorative wallpaper layer --------------------------- */
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
    const onCustom = (e: Event) => {
      const v = (e as CustomEvent).detail as boolean;
      setEnabledState(v);
    };
    window.addEventListener("storage", onStorage);
    window.addEventListener(ENABLE_CHANGE, onCustom);
    return () => {
      window.removeEventListener("storage", onStorage);
      window.removeEventListener(ENABLE_CHANGE, onCustom);
    };
  }, []);

  return { enabled, setEnabled };
}
