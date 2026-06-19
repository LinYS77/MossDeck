/** CSRF double-submit helpers.
 *
 * The backend issues a readable (non-HttpOnly) cookie named `homepage_csrf`.
 * Browser JS reads it here and echoes the value back in the `X-CSRF-Token`
 * header on unsafe methods. The backend's middleware compares them.
 *
 * @see backend README "Frontend API Contract"
 */

/** Default cookie/header names — match the backend defaults (config-overridable). */
export const CSRF_COOKIE_NAME = "homepage_csrf";
export const CSRF_HEADER_NAME = "X-CSRF-Token";

/** Read the current CSRF token from document.cookie (empty string if absent). */
export function readCSRFToken(cookieName = CSRF_COOKIE_NAME): string {
  if (typeof document === "undefined") return "";
  const prefix = `${cookieName}=`;
  const match = document.cookie
    .split(";")
    .map((c) => c.trim())
    .find((c) => c.startsWith(prefix));
  if (!match) return "";
  try {
    return decodeURIComponent(match.slice(prefix.length));
  } catch {
    return match.slice(prefix.length);
  }
}

/** Refresh the CSRF token by calling the backend token endpoint.
 *
 * `GET /api/v1/auth/csrf` is a safe method and is always allowed. The server
 * sets a fresh cookie and returns `{ token, cookieName, headerName }`. We then
 * re-read the cookie so the next unsafe request carries the new value.
 */
export async function refreshCSRFToken(cookieName = CSRF_COOKIE_NAME): Promise<string> {
  await fetch("/api/v1/auth/csrf", { credentials: "include" });
  return readCSRFToken(cookieName);
}
