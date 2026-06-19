/** Auth API: setup / login / logout / me. */

import { request } from "./client";

/** Public user representation returned by auth endpoints. */
export interface UserDTO {
  id: number;
  username: string;
  email?: string;
  displayName?: string;
  role: string;
  status: string;
  lastLoginAt?: string;
  createdAt: string;
}

export interface SetupParams {
  username: string;
  password: string;
  email?: string;
  displayName?: string;
}

/** Current authenticated user. 401 if not signed in. */
export function getMe(signal?: AbortSignal): Promise<UserDTO> {
  return request<UserDTO>("GET", "/api/v1/auth/me", { signal });
}

/** Create the first admin (CSRF-exempt bootstrap; 409 SETUP_DISABLED if done). */
export function setup(params: SetupParams, signal?: AbortSignal): Promise<UserDTO> {
  return request<UserDTO>("POST", "/api/v1/auth/setup", { body: params, signal });
}

/** Sign in (CSRF-exempt; sets session + csrf cookies). 401 on bad credentials,
 * 429 on too many attempts. */
export function login(
  username: string,
  password: string,
  signal?: AbortSignal,
): Promise<UserDTO> {
  return request<UserDTO>("POST", "/api/v1/auth/login", {
    body: { username, password },
    signal,
  });
}

/** Sign out (requires CSRF when a session cookie is present). Clears cookies. */
export function logout(signal?: AbortSignal): Promise<void> {
  return request<void>("POST", "/api/v1/auth/logout", { signal });
}
