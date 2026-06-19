import { useEffect, useState } from "react";
import { Link, Navigate, useLocation, useNavigate } from "react-router-dom";
import { GlassPanel } from "../components/GlassPanel";
import { HomeIcon } from "../components/icons";
import { useAuth, SetupCompletedError } from "../features/auth/AuthProvider";
import * as authApi from "../lib/api/auth";
import { ApiError } from "../lib/api/client";
import { useI18n } from "../i18n/I18nProvider";
import styles from "./LoginPage.module.css";

type Mode = "login" | "setup";

interface LocationState {
  from?: { pathname?: string };
}

export function LoginPage() {
  const { status, login, setup } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const from = (location.state as LocationState | null)?.from?.pathname ?? "/";
  const { t } = useI18n();

  const [serverStatus, setServerStatus] = useState<authApi.AuthStatusDTO | null>(null);
  const [mode, setMode] = useState<Mode>("login");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [setupToken, setSetupToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    void (async () => {
      try {
        const next = await authApi.getAuthStatus(controller.signal);
        if (controller.signal.aborted) return;
        setServerStatus(next);
        setMode(next.initialized ? "login" : "setup");
      } catch {
        if (!controller.signal.aborted) setMode("login");
      }
    })();
    return () => controller.abort();
  }, []);

  if (status === "authenticated") {
    return <Navigate to={from} replace />;
  }

  const canSetup = mode === "setup" && serverStatus?.setupEnabled !== false;

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (busy) return;
    setBusy(true);
    setError(null);
    try {
      if (mode === "login") {
        await login(password);
      } else {
        if (password !== confirmPassword) {
          setError(t("login.errorPasswordMismatch"));
          return;
        }
        await setup({
          password,
          confirmPassword,
          setupToken: setupToken.trim() || undefined,
        });
      }
      navigate(from, { replace: true });
    } catch (err) {
      setError(errorMessage(err, t));
    } finally {
      setBusy(false);
    }
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
          {mode === "login" ? t("login.welcomeBack") : t("login.createPasswordTitle")}
        </h1>
        <p className={styles.subtitle}>
          {mode === "login" ? t("login.signInTo") : t("login.createPasswordPrompt")}
        </p>

        <form className={styles.form} onSubmit={submit}>
          {mode === "setup" && serverStatus?.setupTokenRequired ? (
            <label className={styles.field}>
              <span className={styles.label}>{t("login.setupToken")}</span>
              <input
                type="password"
                className={styles.input}
                placeholder="••••••••"
                value={setupToken}
                onChange={(e) => setSetupToken(e.target.value)}
                autoComplete="one-time-code"
                autoFocus
                required
              />
            </label>
          ) : null}

          <label className={styles.field}>
            <span className={styles.label}>{t("login.password")}</span>
            <input
              type="password"
              className={styles.input}
              placeholder="••••••••••••"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete={mode === "login" ? "current-password" : "new-password"}
              autoFocus={mode === "login" || !serverStatus?.setupTokenRequired}
              required
            />
          </label>

          {mode === "setup" ? (
            <>
              <label className={styles.field}>
                <span className={styles.label}>{t("login.confirmPassword")}</span>
                <input
                  type="password"
                  className={styles.input}
                  placeholder="••••••••••••"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  autoComplete="new-password"
                  required
                />
              </label>
              <p className={styles.requirements}>{t("login.passwordRules")}</p>
            </>
          ) : null}

          {error ? (
            <p className={styles.error} role="alert">
              {error}
            </p>
          ) : null}

          <button type="submit" className={styles.submit} disabled={busy || (!canSetup && mode === "setup")}>
            {busy
              ? mode === "login"
                ? t("login.signingIn")
                : t("login.creating")
              : mode === "login"
                ? t("login.unlock")
                : t("login.createPassword")}
          </button>
        </form>

        <p className={styles.hint}>
          {mode === "login"
            ? t("login.hintLogin")
            : canSetup
              ? t("login.hintSetup")
              : t("login.hintSetupDisabled")}
        </p>
      </GlassPanel>
    </div>
  );
}

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
      case "SETUP_TOKEN_INVALID":
        return t("login.errorSetupToken");
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
