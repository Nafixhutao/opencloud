import { headers } from 'next/headers';

import { auth } from '@/lib/auth';

interface ApiErrorBody {
  error: {
    code: string;
    message: string;
    details?: { field: string; issue: string }[];
  };
}

export class ApiError extends Error {
  status: number;
  code: string;
  details?: { field: string; issue: string }[];

  constructor(
    message: string,
    status: number,
    code: string,
    details?: { field: string; issue: string }[],
  ) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.details = details;
  }

  static fromResponse(res: Response, body: unknown): ApiError {
    const errBody = body as ApiErrorBody | null;
    const message = errBody?.error?.message ?? `API ${res.status}`;
    const details = errBody?.error?.details;
    return new ApiError(message, res.status, errBody?.error?.code ?? 'unknown', details);
  }
}

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
  if (!token || typeof token !== 'string') {
    throw new ApiError('UNAUTHENTICATED', 401, 'unauthorized');
  }

  const hdrs = new Headers(init.headers);
  hdrs.set('Authorization', `Bearer ${token}`);
  if (!hdrs.has('Content-Type') && init.body) {
    hdrs.set('Content-Type', 'application/json');
  }

  const response = await fetch(`${apiURL}${path}`, {
    ...init,
    headers: hdrs,
    cache: 'no-store',
  });

  if (!response.ok) {
    try {
      const body = await response.json().catch(() => null);
      throw ApiError.fromResponse(response, body);
    } catch {
      throw new ApiError(`API ${response.status}`, response.status, 'unknown');
    }
  }

  return response;
}

export async function apiJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await apiFetch(path, init);
  const body = (await res.json().catch(() => null)) as T | ApiErrorBody | null;
  if (!res.ok) {
    throw ApiError.fromResponse(res, body);
  }
  return body as T;
}
