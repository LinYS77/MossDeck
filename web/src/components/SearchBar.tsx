import { useEffect, useId, useRef, useState } from "react";
import { cn } from "../lib/cn";
import type { SearchEngineId } from "../lib/types";
import { SEARCH_ENGINES } from "../features/home/mockData";
import {
  ArrowUpIcon,
  ChevronDownIcon,
  GlobeIcon,
  SearchIcon,
} from "./icons";
import styles from "./SearchBar.module.css";

/** Large hero search bar with an engine selector and submit affordance. */
export function SearchBar() {
  const [value, setValue] = useState("");
  const [engineId, setEngineId] = useState<SearchEngineId>("duckduckgo");
  const [menuOpen, setMenuOpen] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const wrapRef = useRef<HTMLDivElement>(null);
  const menuId = useId();

  const engine = SEARCH_ENGINES.find((e) => e.id === engineId) ?? SEARCH_ENGINES[0];

  // Close the engine menu on outside click or Escape so it never lingers open
  // over the content it now floats above.
  useEffect(() => {
    if (!menuOpen) return;
    const onPointerDown = (e: MouseEvent) => {
      if (!wrapRef.current?.contains(e.target as Node)) setMenuOpen(false);
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setMenuOpen(false);
        inputRef.current?.focus();
      }
    };
    document.addEventListener("mousedown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [menuOpen]);

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    const q = value.trim();
    if (!q) return;
    // For a prototype, navigate directly. A real build could route internally.
    window.location.href = engine.url(q);
  };

  return (
    <form className={styles.form} onSubmit={submit} role="search">
      <div className={styles.bar}>
        <SearchIcon className={styles.searchIcon} />

        <input
          ref={inputRef}
          type="text"
          className={styles.input}
          placeholder={`Search ${engine.label} or type a URL`}
          value={value}
          onChange={(e) => setValue(e.target.value)}
          autoComplete="off"
          spellCheck={false}
          aria-label="Search"
        />

        {/* Engine selector */}
        <div className={styles.engineWrap} ref={wrapRef}>
          <button
            type="button"
            className={styles.engineBtn}
            onClick={() => setMenuOpen((o) => !o)}
            aria-haspopup="menu"
            aria-expanded={menuOpen}
            aria-controls={menuId}
          >
            <GlobeIcon className={styles.engineIcon} />
            <span className={styles.engineLabel}>{engine.label}</span>
            <ChevronDownIcon className={cn(styles.chev, menuOpen && styles.chevOpen)} />
          </button>

          {menuOpen ? (
            <div className={styles.menu} id={menuId} role="menu">
              {SEARCH_ENGINES.map((e) => (
                <button
                  key={e.id}
                  type="button"
                  role="menuitemradio"
                  aria-checked={e.id === engineId}
                  className={cn(styles.menuItem, e.id === engineId && styles.menuItemActive)}
                  onClick={() => {
                    setEngineId(e.id);
                    inputRef.current?.focus();
                  }}
                >
                  {e.label}
                </button>
              ))}
            </div>
          ) : null}
        </div>

        <button
          type="submit"
          className={styles.submit}
          aria-label="Search"
          disabled={!value.trim()}
        >
          <ArrowUpIcon />
        </button>
      </div>
    </form>
  );
}
