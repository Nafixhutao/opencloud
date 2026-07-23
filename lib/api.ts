import { headers } from 'next/headers';

import { auth } from '@/lib/auth';

/**
 * Server-side helper: exchange the better-auth session cookie for a JWT
 * carrying trusted account_id + role claims, then call the Go API.
 * Import only from Server Components / Route Handlers / Server Actions.
 */
export async function apiFetch(path: string, init: RequestInit = {}): Promise<Response> {
  const apiURL = process.env.API_URL ?? 'http://127.0.0.1:8080';
  const h = await headers();

  const tokenRes = await auth.api.getToken({ headers: h });
  const token = tokenRes?.token;
  if (!token) {
    throw new Error('UNAUTHENTICATED');
  }

  const hdrs = new Headers(init.headers);
  hdrs.set('Authorization', `Bearer ${token}`);
  if (!hdrs.has('Content-Type') && init.body) {
    hdrs.set('Content-Type', 'application/json');
  }

  return fetch(`${apiURL}${path}`, {
    ...init,
    headers: hdrs,
    cache: 'no-store',
  });
}

export type ApiErrorBody = {
  error: { code: string; message: string; details?: { field: string; issue: string }[] };
};

export async function apiJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await apiFetch(path, init);
  const body = (await res.json().catch(() => null)) as T | ApiErrorBody | null;
  if (!res.ok) {
    const err = body as ApiErrorBody | null;
    const message = err?.error?.message ?? `API ${res.status}`;
    const error = new Error(message) as Error & { status?: number; code?: string };
    error.status = res.status;
    error.code = err?.error?.code;
    throw error;
  }
  return body as T;
}
