import { Navigate, useLocation } from "react-router-dom";
import { useAuth } from "../features/auth/AuthProvider";
import { AuthLoading } from "./AuthLoading";
import styles from "./AuthLoading.module.css";

/** Route guard: redirects unauthenticated users to /login (remembering the
 *  intended destination). Renders a glass loading screen while the session is
 *  being probed. */
export function RequireAuth({ children }: { children: React.ReactNode }) {
  const { status } = useAuth();
  const location = useLocation();

  if (status === "loading") {
    return (
      <div className={styles.screen}>
        <AuthLoading />
      </div>
    );
  }

  if (status !== "authenticated") {
    return <Navigate to="/login" replace state={{ from: location }} />;
  }

  return <>{children}</>;
}
