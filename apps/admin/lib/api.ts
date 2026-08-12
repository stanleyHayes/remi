"use client";

import { apiBaseUrl as API_URL } from "./env";

const TOKEN_KEY = "remi_admin_token";
const USER_KEY = "remi_admin_user";

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

export function getToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem(TOKEN_KEY);
}

export function getStoredUser(): AdminUser | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = localStorage.getItem(USER_KEY);
    return raw ? (JSON.parse(raw) as AdminUser) : null;
  } catch {
    return null;
  }
}

export function setSession(token: string, user: AdminUser) {
  localStorage.setItem(TOKEN_KEY, token);
  localStorage.setItem(USER_KEY, JSON.stringify(user));
}

export function clearSession() {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(USER_KEY);
}

export interface AdminUser {
  id?: string;
  email: string;
  name?: string;
  role?: "super-admin" | "editor" | "viewer" | string;
	phone?: string;
	title?: string;
	bio?: string;
	avatarUrl?: string;
	mfaEnabled?: boolean;
	lastLoginAt?: string;
	preferences?: AdminPreferences;
}

export interface AdminPreferences {
	density?: "comfortable" | "compact";
	theme?: "light" | "dark" | "system";
	emailDigest?: boolean;
	reducedMotion?: boolean;
	timezone?: string;
}

interface ApiOptions {
  method?: string;
  body?: unknown;
  headers?: Record<string, string>;
}

export async function api<T = unknown>(path: string, options: ApiOptions = {}): Promise<T> {
  const headers: Record<string, string> = { ...(options.headers || {}) };
  if (options.body !== undefined) headers["Content-Type"] = "application/json";
  const token = getToken();
  if (token) headers["Authorization"] = `Bearer ${token}`;
  if (path.startsWith("/api/chms/") && typeof crypto !== "undefined") {
    headers["X-Request-ID"] ||= crypto.randomUUID();
    const method = (options.method || "GET").toUpperCase();
    if (method === "POST") headers["Idempotency-Key"] ||= crypto.randomUUID();
  }

  const res = await fetch(`${API_URL}${path}`, {
    method: options.method ?? "GET",
    headers,
    body: options.body !== undefined ? JSON.stringify(options.body) : undefined,
  });

  if (res.status === 204) return null as T;

  let data: unknown = null;
  const text = await res.text();
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = null;
    }
  }

  if (!res.ok) {
    const errorValue = data && typeof data === "object" && "error" in data ? (data as { error: unknown }).error : null;
    const message = typeof errorValue === "string" ? errorValue : errorValue && typeof errorValue === "object" && "message" in errorValue && typeof (errorValue as {message:unknown}).message === "string" ? (errorValue as {message:string}).message : `Request failed (${res.status})`;
    if (res.status === 401) {
      clearSession();
      const isSignInRequest = path === "/api/auth/login" || path === "/api/auth/mfa/verify";
      if (!isSignInRequest && typeof window !== "undefined" && !window.location.pathname.startsWith("/login")) {
        window.location.href = "/login";
      }
      throw new ApiError(401, isSignInRequest ? message : "Session expired. Please sign in again.");
    }
    throw new ApiError(res.status, message);
  }

  return data as T;
}

export async function apiForm<T = unknown>(path: string, form: FormData, method = "POST"): Promise<T> {
  const headers: Record<string, string> = {};
  const token = getToken();
  if (token) headers.Authorization = `Bearer ${token}`;
  if (path.startsWith("/api/chms/") && typeof crypto !== "undefined") {
    headers["X-Request-ID"] = crypto.randomUUID();
    if (method.toUpperCase() === "POST") headers["Idempotency-Key"] = crypto.randomUUID();
  }
  const response = await fetch(`${API_URL}${path}`, { method, headers, body: form });
  const text = await response.text();
  let data: unknown = null;
  if (text) { try { data = JSON.parse(text); } catch { data = null; } }
  if (!response.ok) {
    const errorValue = data && typeof data === "object" && "error" in data ? (data as {error:unknown}).error : null;
    const message = typeof errorValue === "string" ? errorValue : errorValue && typeof errorValue === "object" && "message" in errorValue && typeof (errorValue as {message:unknown}).message === "string" ? (errorValue as {message:string}).message : `Request failed (${response.status})`;
    if (response.status === 401) clearSession();
    throw new ApiError(response.status, message);
  }
  return data as T;
}

export async function apiDownload(path: string, fallbackName: string): Promise<void> {
  const headers: Record<string, string> = {};
  const token = getToken();
  if (token) headers.Authorization = `Bearer ${token}`;
  const response = await fetch(`${API_URL}${path}`, { headers, cache: "no-store" });
  if (!response.ok) {
    let message = `Download failed (${response.status})`;
    try {
      const data = await response.json() as { error?: string | { message?: string } };
      message = typeof data.error === "string" ? data.error : data.error?.message || message;
    } catch {}
    if (response.status === 401) clearSession();
    throw new ApiError(response.status, message);
  }
  const blob = await response.blob();
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = fallbackName;
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(url);
}

export async function apiDownloadPost(path: string, body: unknown, fallbackName: string): Promise<void> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  const token = getToken();
  if (token) headers.Authorization = `Bearer ${token}`;
  if (typeof crypto !== "undefined") headers["X-Request-ID"] = crypto.randomUUID();
  const response = await fetch(`${API_URL}${path}`, { method: "POST", headers, body: JSON.stringify(body), cache: "no-store" });
  if (!response.ok) {
    let message = `Export failed (${response.status})`;
    try { const data = await response.json() as {error?:string|{message?:string}}; message = typeof data.error === "string" ? data.error : data.error?.message || message; } catch {}
    throw new ApiError(response.status, message);
  }
  const blob = await response.blob();
  const url = URL.createObjectURL(blob); const anchor = document.createElement("a");
  anchor.href = url; anchor.download = fallbackName; document.body.appendChild(anchor); anchor.click(); anchor.remove(); URL.revokeObjectURL(url);
}

/** List endpoints may return a bare array or a paginated { items } envelope. */
export function asList<T = Record<string, unknown>>(data: unknown): T[] {
  if (Array.isArray(data)) return data as T[];
  if (data && typeof data === "object" && Array.isArray((data as { items?: unknown }).items)) {
    return (data as { items: T[] }).items;
  }
  return [];
}
