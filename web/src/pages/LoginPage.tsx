import { useState } from "react";
import { Link, Navigate, useLocation, useNavigate } from "react-router-dom";
import { GlassPanel } from "../components/GlassPanel";
import { HomeIcon } from "../components/icons";
import { useAuth, SetupCompletedError } from "../features/auth/AuthProvider";
import { ApiError } from "../lib/api/client";
import { useI18n } from "../i18n/I18nProvider";
import styles from "./LoginPage.module.css";

type Mode = "login" | "setup";

const setupVisible = import.meta.env.VITE_ENABLE_SETUP === "true" || import.meta.env.DEV;

interface LocationState {
  from?: { pathname?: string };
}

/** Auth entry point. Supports sign-in and first-run admin creation. When
 *  already authenticated, bounces to the intended (or home) destination. */
export function LoginPage() {
  const { status, login, setup } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const from = (location.state as LocationState | null)?.from?.pathname ?? "/";
  const { t } = useI18n();

  const [mode, setMode] = useState<Mode>("login");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (status === "authenticated") {
    return <Navigate to={from} replace />;
  }

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      if (mode === "login") {
        await login(username.trim(), password);
      } else {
        await setup({
          username: username.trim(),
          password,
          displayName: displayName.trim() || undefined,
        });
      }
      navigate(from, { replace: true });
    } catch (err) {
      setError(errorMessage(err, t));
    } finally {
      setBusy(false);
    }
  };

  const switchMode = (next: Mode) => {
    if (next === "setup" && !setupVisible) return;
    setMode(next);
    setError(null);
  };

  return (
    <div className={styles.page}>
      <Link to="/" className={styles.backHome}>
        <HomeIcon className={styles.backIcon} />
        {t("login.backHome")}
      </Link>

      <GlassPanel variant="strong" className={styles.card}>
        <div className={styles.brand}>
          <span className={styles.brandMark}>
            <HomeIcon />
          </span>
        </div>
        <h1 className={styles.title}>
          {mode === "login" ? t("login.welcomeBack") : t("login.createHomepage")}
        </h1>
        <p className={styles.subtitle}>
          {mode === "login" ? t("login.signInTo") : t("login.setupPrompt")}
        </p>

        {setupVisible ? (
          <div className={styles.tabs} role="tablist" aria-label="Auth mode">
            <button
              type="button"
              role="tab"
              aria-selected={mode === "login"}
              className={`${styles.tab} ${mode === "login" ? styles.tabActive : ""}`}
              onClick={() => switchMode("login")}
            >
              {t("login.signIn")}
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={mode === "setup"}
              className={`${styles.tab} ${mode === "setup" ? styles.tabActive : ""}`}
              onClick={() => switchMode("setup")}
            >
              {t("login.firstTimeSetup")}
            </button>
          </div>
        ) : null}

        <form className={styles.form} onSubmit={submit}>
          {mode === "setup" ? (
            <label className={styles.field}>
              <span className={styles.label}>{t("login.displayName")}</span>
              <input
                type="text"
                className={styles.input}
                placeholder="Winnie"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                autoComplete="name"
              />
            </label>
          ) : null}

          <label className={styles.field}>
            <span className={styles.label}>{t("login.username")}</span>
            <input
              type="text"
              className={styles.input}
              placeholder="admin"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              autoComplete="username"
              autoFocus
              required
            />
          </label>

          <label className={styles.field}>
            <span className={styles.label}>{t("login.password")}</span>
            <input
              type="password"
              className={styles.input}
              placeholder="••••••••"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete={mode === "login" ? "current-password" : "new-password"}
              required
            />
          </label>

          {error ? (
            <p className={styles.error} role="alert">
              {error}
            </p>
          ) : null}

          <button type="submit" className={styles.submit} disabled={busy}>
            {busy
              ? mode === "login"
                ? t("login.signingIn")
                : t("login.creating")
              : mode === "login"
                ? t("login.signIn")
                : t("login.createAccount")}
          </button>
        </form>

        <p className={styles.hint}>
          {mode === "login"
            ? setupVisible
              ? t("login.hintFirstRun")
              : t("login.hintLogin")
            : t("login.hintSetup")}
        </p>
      </GlassPanel>
    </div>
  );
}

/** Map an API error to a friendly message for the auth form. */
function errorMessage(err: unknown, t: (k: string) => string): string {
  if (err instanceof SetupCompletedError) {
    return err.reason === "disabled"
      ? t("login.errorSetupDisabledServer")
      : t("login.errorSetupDisabled");
  }
  if (err instanceof ApiError) {
    switch (err.code) {
      case "UNAUTHORIZED":
        return t("login.errorInvalid");
      case "SETUP_DISABLED":
        return t("login.errorSetupDisabled");
      case "TOO_MANY_REQUESTS":
        return t("login.errorTooMany");
      case "BAD_REQUEST":
        return err.message || t("login.errorCheckInput");
      default:
        return err.message || t("error.internal");
    }
  }
  if (err instanceof Error && err.message) return err.message;
  return t("login.errorServer");
}
