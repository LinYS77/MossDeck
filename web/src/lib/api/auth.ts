/** Auth API: status / setup / login / logout / me. */

import { request } from "./client";

export interface UserDTO {
  id: number;
  lastLoginAt?: string;
  createdAt: string;
}

export interface AuthStatusDTO {
  initialized: boolean;
  setupEnabled: boolean;
  setupTokenRequired: boolean;
}

export interface SetupParams {
  password: string;
  confirmPassword: string;
  setupToken?: string;
}

export function getAuthStatus(signal?: AbortSignal): Promise<AuthStatusDTO> {
  return request<AuthStatusDTO>("GET", "/api/v1/auth/status", { signal });
}

export function getMe(signal?: AbortSignal): Promise<UserDTO> {
  return request<UserDTO>("GET", "/api/v1/auth/me", { signal });
}

export function setup(params: SetupParams, signal?: AbortSignal): Promise<UserDTO> {
  return request<UserDTO>("POST", "/api/v1/auth/setup", { body: params, signal });
}

export function login(password: string, signal?: AbortSignal): Promise<UserDTO> {
  return request<UserDTO>("POST", "/api/v1/auth/login", {
    body: { password },
    signal,
  });
}

export function logout(signal?: AbortSignal): Promise<void> {
  return request<void>("POST", "/api/v1/auth/logout", { signal });
}
