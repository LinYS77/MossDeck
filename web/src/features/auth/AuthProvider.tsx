import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import * as authApi from "../../lib/api/auth";
import {
  ApiError,
  setUnauthorizedHandler,
} from "../../lib/api/client";

export type AuthStatus = "loading" | "authenticated" | "unauthenticated" | "error";

export interface AuthContextValue {
  user: authApi.UserDTO | null;
  status: AuthStatus;
  error: string | null;
  login: (password: string) => Promise<void>;
  setup: (params: authApi.SetupParams) => Promise<void>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

const SETUP_DONE_CODE = "SETUP_DISABLED";

export class SetupCompletedError extends Error {
  readonly reason: "disabled" | "completed";

  constructor(reason: "disabled" | "completed") {
    super(reason === "disabled" ? "Setup is disabled" : "Setup has already been completed");
    this.name = "SetupCompletedError";
    this.reason = reason;
  }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<authApi.UserDTO | null>(null);
  const [status, setStatus] = useState<AuthStatus>("loading");
  const [error, setError] = useState<string | null>(null);

  const clearRef = useRef<() => void>(() => {});
  clearRef.current = () => {
    setUser(null);
    setStatus("unauthenticated");
  };

  useEffect(() => {
    setUnauthorizedHandler(() => clearRef.current());
    return () => setUnauthorizedHandler(null);
  }, []);

  const refresh = useCallback(async () => {
    setStatus("loading");
    setError(null);
    try {
      const me = await authApi.getMe();
      setUser(me);
      setStatus("authenticated");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setUser(null);
        setStatus("unauthenticated");
      } else {
        setUser(null);
        setStatus("error");
        setError(err instanceof Error ? err.message : "Unable to reach the server");
      }
    }
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void (async () => {
      try {
        const me = await authApi.getMe(controller.signal);
        setUser(me);
        setStatus("authenticated");
      } catch (err) {
        if (controller.signal.aborted) return;
        if (err instanceof ApiError && err.status === 401) {
          setStatus("unauthenticated");
        } else {
          setStatus("error");
          setError(err instanceof Error ? err.message : "Unable to reach the server");
        }
      }
    })();
    return () => controller.abort();
  }, []);

  const login = useCallback(async (password: string) => {
    setError(null);
    const me = await authApi.login(password);
    setUser(me);
    setStatus("authenticated");
  }, []);

  const setup = useCallback(async (params: authApi.SetupParams) => {
    setError(null);
    try {
      await authApi.setup(params);
    } catch (err) {
      if (err instanceof ApiError && err.code === SETUP_DONE_CODE) {
        throw new SetupCompletedError(err.status === 403 ? "disabled" : "completed");
      }
      throw err;
    }
    const me = await authApi.login(params.password);
    setUser(me);
    setStatus("authenticated");
  }, []);

  const logout = useCallback(async () => {
    try {
      await authApi.logout();
    } catch {
      // Drop local state even if the server already cleared the session.
    } finally {
      setUser(null);
      setStatus("unauthenticated");
      setError(null);
    }
  }, []);

  const value = useMemo<AuthContextValue>(
    () => ({ user, status, error, login, setup, logout, refresh }),
    [user, status, error, login, setup, logout, refresh],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within <AuthProvider>");
  return ctx;
}
