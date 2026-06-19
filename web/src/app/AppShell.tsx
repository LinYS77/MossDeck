import { useEffect } from "react";
import { BrowserRouter, Navigate, Outlet, Route, Routes } from "react-router-dom";
import { useWallpaper, useWallpaperEnabled } from "../lib/wallpapers";
import { AuthProvider } from "../features/auth/AuthProvider";
import { I18nProvider } from "../i18n/I18nProvider";
import { RequireAuth } from "../pages/RequireAuth";
import { HomePage } from "../pages/HomePage";
import { BookmarksPage } from "../pages/BookmarksPage";
import { ReadLaterPage } from "../pages/ReadLaterPage";
import { LoginPage } from "../pages/LoginPage";
import { NotFound } from "../pages/NotFound";
import { SettingsPage } from "../pages/SettingsPage";
import { TopBar } from "../components/TopBar";

const RAIL_ITEMS = [
  "Homepage",
  "Bookmarks",
  "Read Later",
  "Settings",
  "Glass UI",
  "Local-first",
];

function AppFrame() {
  return (
    <div className="app-frame">
      <TopBar />
      <Outlet />
    </div>
  );
}

/** Application shell: paper + grid + cursor-beam canvas, an OPTIONAL
 *  decorative wallpaper behind it, primary routes, and a bottom marquee.
 *  Auth guards the home + management routes; /login is public. */
export function AppShell() {
  const { wallpaper } = useWallpaper();
  const { enabled } = useWallpaperEnabled();
  const wallpaperOn = enabled && !!wallpaper;

  // Drive both pointer treatments: the vivid cyan beam on the plain paper
  // canvas, and the restrained shadow lens on the wallpaper canvas.
  useEffect(() => {
    const root = document.documentElement;
    let raf = 0;
    const onMove = (e: PointerEvent) => {
      if (raf) return;
      raf = requestAnimationFrame(() => {
        raf = 0;
        const x = ((e.clientX / window.innerWidth) * 100).toFixed(1);
        const y = ((e.clientY / window.innerHeight) * 100).toFixed(1);
        root.style.setProperty("--mx", `${x}%`);
        root.style.setProperty("--my", `${y}%`);
      });
    };
    window.addEventListener("pointermove", onMove, { passive: true });
    return () => {
      window.removeEventListener("pointermove", onMove);
      if (raf) cancelAnimationFrame(raf);
    };
  }, [wallpaperOn]);

  return (
    <>
      {wallpaperOn && wallpaper ? (
        <div
          className="app-bg"
          style={{ backgroundImage: `url(${wallpaper.src})` }}
          aria-hidden
        />
      ) : null}

      {/* Restrained cursor shadow on wallpaper mode — replaces the failed
          distortion experiment with a quiet depth cue. */}
      {wallpaperOn ? <div className="app-bg__shadow" aria-hidden /> : null}
      {/* Dense analogue-film grain over the wallpaper. */}
      {wallpaperOn ? <div className="app-bg__grain" aria-hidden /> : null}
      {/* Grid + cursor beam on the PLAIN paper canvas. When a wallpaper is on:
          grid lines, static brand glows, and cursor beam are all disabled. */}
      <div
        className="app-bg__grid"
        style={!wallpaperOn
          ? undefined
          : ({
              "--bg-beam-current": "var(--bg-beam-wallpaper)",
              "--bg-grid": "none",
              "--bg-glow": "none",
            } as React.CSSProperties)}
        aria-hidden
      />

      <AuthProvider>
        <I18nProvider>
          <BrowserRouter>
            <Routes>
              <Route path="/login" element={<LoginPage />} />
              <Route
                element={
                  <RequireAuth>
                    <AppFrame />
                  </RequireAuth>
                }
              >
                <Route path="/" element={<HomePage />} />
                <Route path="/bookmarks" element={<BookmarksPage />} />
                <Route path="/read-later" element={<ReadLaterPage />} />
                <Route path="/settings" element={<SettingsPage />} />
              </Route>
              <Route path="/home" element={<Navigate to="/" replace />} />
              <Route path="*" element={<NotFound />} />
            </Routes>
          </BrowserRouter>
        </I18nProvider>
      </AuthProvider>

      {/* Bottom marquee ticker. */}
      <div className="app-rail" aria-hidden>
        <div className="app-rail__track">
          {Array.from({ length: 2 }).map((_, dup) => (
            <span className="app-rail__group" key={dup}>
              {RAIL_ITEMS.map((label, i) => (
                <span key={label}>
                  {label}
                  <span className="app-rail__dot" style={{ marginLeft: 8 }}>
                    {i === RAIL_ITEMS.length - 1 ? "✦" : "/"}
                  </span>
                </span>
              ))}
            </span>
          ))}
        </div>
      </div>
    </>
  );
}
