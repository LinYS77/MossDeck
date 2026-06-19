/** Small glass loading affordance shown while the session is probed or data
 *  is fetched. Keeps the premium feel instead of a plain "Loading…" string. */
export function AuthLoading({ label = "Loading" }: { label?: string }) {
  return (
    <div className="app-shell-loading" role="status" aria-live="polite">
      <span className="app-shell-loading__spinner" aria-hidden />
      <span className="app-shell-loading__text">{label}…</span>
    </div>
  );
}
