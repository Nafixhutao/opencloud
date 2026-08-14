import { NextResponse } from 'next/server';
import type { ZodTypeAny, infer as zodInfer } from 'zod';

import { apiFetch, ApiError } from '@/lib/api';

/**
 * apiErrorToResponse translates a structured ApiError into the client-facing
 * JSON envelope. This is the single seam where the error shape contract lives
 * — routes and proxyAPI call this instead of string-matching error.message.
 *
 * Auth errors (status 401) are mapped to the enumeration-safe UNAUTHENTICATED
 * code the client expects. Upstream 4xx/5xx preserve their status, code, and
 * details so validation errors reach the client intact.
 */
export function apiErrorToResponse(error: ApiError): NextResponse {
  if (error.status === 401) {
    return NextResponse.json(
      { error: { code: 'UNAUTHENTICATED', message: 'Sign in required' } },
      { status: 401 },
    );
  }
  return NextResponse.json(
    {
      error: {
        code: error.code,
        message: error.message,
        ...(error.details ? { details: error.details } : {}),
      },
    },
    { status: error.status },
  );
}

/**
 * proxyAPI forwards a request to the Go API through the authenticated BFF
 * layer. It preserves a curated header allowlist (cache-control, pragma,
 * retry-after, x-ratelimit-limit) and strips Authorization. Errors are
 * translated via apiErrorToResponse — no string matching, no status flattening.
 */
export async function proxyAPI(
  path: string,
  init: RequestInit,
  fallbackMessage: string,
): Promise<NextResponse> {
  try {
    const response = await apiFetch(path, init);
    let proxied: NextResponse;
    // 204/304 must not carry a body — NextResponse.json() throws on those
    // statuses, and forcing a literal `null` body on other empty 2xx responses
    // corrupts the contract. Pass them through untouched instead.
    if (response.status === 204 || response.status === 304 || response.body === null) {
      proxied = new NextResponse(null, { status: response.status });
    } else {
      const body = await response.json().catch(() => null);
      proxied = NextResponse.json(body, { status: response.status });
    }
    for (const name of ['cache-control', 'pragma', 'retry-after', 'x-ratelimit-limit']) {
      const value = response.headers.get(name);
      if (value) {
        proxied.headers.set(name, value);
      }
    }
    return proxied;
  } catch (error) {
    if (error instanceof ApiError) {
      return apiErrorToResponse(error);
    }
    return NextResponse.json(
      { error: { code: 'INTERNAL', message: fallbackMessage } },
      { status: 502 },
    );
  }
}

/**
 * withValidatedBody is the deep helper for route handlers that need to parse
 * and validate a JSON body before calling the Go API. It absorbs the
 * parse → safeParse → VALIDATION_FAILED mapping that was previously copied
 * into 7 route files.
 *
 * On invalid JSON or schema failure, it returns a 422 VALIDATION_FAILED
 * response with structured field details. On success, it calls the handler
 * with the typed parsed body. Any ApiError thrown by the handler is
 * translated via apiErrorToResponse.
 */
export async function withValidatedBody<S extends ZodTypeAny>(
  request: Request,
  schema: S,
  handler: (body: zodInfer<S>) => Promise<NextResponse>,
  opts: { message?: string } = {},
): Promise<NextResponse> {
  let json: unknown;
  try {
    json = await request.json();
  } catch {
    return NextResponse.json(
      { error: { code: 'VALIDATION_FAILED', message: 'Invalid JSON body' } },
      { status: 422 },
    );
  }

  const parsed = schema.safeParse(json);
  if (!parsed.success) {
    return NextResponse.json(
      {
        error: {
          code: 'VALIDATION_FAILED',
          message: opts.message ?? 'Check the provided details and try again.',
          details: parsed.error.issues.map((issue) => ({
            field: issue.path.join('.') || 'root',
            issue: issue.message,
          })),
        },
      },
      { status: 422 },
    );
  }

  try {
    return await handler(parsed.data);
  } catch (error) {
    if (error instanceof ApiError) {
      return apiErrorToResponse(error);
    }
    throw error;
  }
}
