/** Unified API client.
 *
 * Responsibilities:
 *   - Always send cookies (`credentials: "include"`).
 *   - JSON-encode request bodies and parse the standard envelope
 *     `{ data, error, requestId }`, returning `data` on success.
 *   - Attach `X-CSRF-Token` (read from the cookie) on unsafe methods.
 *   - Map failures to a typed `ApiError` (status + machine code + message).
 *   - On `401`: clear local auth state via a registered unauthorized handler.
 *   - On `403 CSRF_INVALID`: refresh the CSRF token once, then retry the
 *     original request a single time.
 *
 * Feature modules (`auth.ts`, `bookmarks.ts`, `readLater.ts`) build on this
 * `request` primitive and stay thin.
 */

import {
  CSRF_HEADER_NAME,
  readCSRFToken,
  refreshCSRFToken,
} from "./csrf";

/** Structured error carrying the backend's machine-readable code. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId?: string;

  constructor(status: number, code: string, message: string, requestId?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.requestId = requestId;
  }
}

type UnsafeMethod = "POST" | "PUT" | "PATCH" | "DELETE";
const UNSAFE = new Set<UnsafeMethod>(["POST", "PUT", "PATCH", "DELETE"]);

/** A handler invoked once when any request returns 401, so the auth layer can
 * clear local state. Registered by AuthProvider. */
type UnauthorizedHandler = () => void;
let unauthorizedHandler: UnauthorizedHandler | null = null;

export function setUnauthorizedHandler(handler: UnauthorizedHandler | null): void {
  unauthorizedHandler = handler;
}

/** Permitted scalar values for a query-string parameter. */
export type QueryValue = string | number | boolean | null | undefined;

export interface RequestOptions {
  /** Query-string params; falsy values are skipped. Interfaces used here should
   *  include an index signature so they satisfy this record type. */
  query?: Record<string, QueryValue>;
  /** JSON-serializable request body (sets Content-Type: application/json). */
  body?: unknown;
  /** Multipart form body (e.g. file upload). When set, the browser assigns the
   *  multipart Content-Type/boundary — do NOT set it manually. */
  form?: FormData;
  signal?: AbortSignal;
}

interface InternalOptions extends RequestOptions {
  /** Internal: prevents infinite CSRF retry loops (one retry max). */
  _isRetry?: boolean;
}

function buildURL(path: string, query?: RequestOptions["query"]): string {
  if (!query) return path;
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined || value === null || value === "") continue;
    params.set(key, String(value));
  }
  const qs = params.toString();
  return qs ? `${path}?${qs}` : path;
}

async function parseEnvelope(res: Response): Promise<ApiEnvelope> {
  const text = await res.text();
  if (!text) return {};
  try {
    return JSON.parse(text) as ApiEnvelope;
  } catch {
    return {};
  }
}

/** The core request primitive. Returns the unwrapped `data` field. */
export async function request<T>(
  method: string,
  path: string,
  opts: RequestOptions = {},
): Promise<T> {
  const internal = opts as InternalOptions;
  const url = buildURL(path, opts.query);

  const headers: Record<string, string> = {};
  let body: BodyInit | undefined;
  if (opts.form !== undefined) {
    // Let fetch set the multipart Content-Type with the boundary.
    body = opts.form;
  } else if (opts.body !== undefined) {
    headers["Content-Type"] = "application/json";
    body = JSON.stringify(opts.body);
  }
  if (UNSAFE.has(method.toUpperCase() as UnsafeMethod)) {
    const token = readCSRFToken();
    if (token) headers[CSRF_HEADER_NAME] = token;
  }

  const res = await fetch(url, {
    method,
    headers,
    body,
    credentials: "include",
    signal: opts.signal,
  });

  const envelope = await parseEnvelope(res);

  if (!res.ok) {
    const code = envelope.error?.code ?? "HTTP_ERROR";
    const message = envelope.error?.message ?? res.statusText;

    // 401 → session is gone/invalid. Notify the auth layer to clear state.
    if (res.status === 401) {
      unauthorizedHandler?.();
    }

    // 403 CSRF_INVALID → refresh token and retry the unsafe request once.
    if (res.status === 403 && code === "CSRF_INVALID" && !internal._isRetry) {
      try {
        await refreshCSRFToken();
      } catch {
        /* fall through to throw; the retry below still happens */
      }
      return request<T>(method, path, { ...opts, _isRetry: true } as InternalOptions);
    }

    throw new ApiError(res.status, code, message, envelope.requestId);
  }

  return envelope.data as T;
}

/** Shape of the uniform response envelope (subset). */
interface ApiEnvelope {
  data?: unknown;
  error?: { code: string; message: string; details?: unknown };
  requestId?: string;
}
