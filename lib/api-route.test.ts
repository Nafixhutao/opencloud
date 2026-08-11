import { afterEach, describe, expect, it, vi } from 'vitest';

const { apiFetchMock } = vi.hoisted(() => ({ apiFetchMock: vi.fn() }));

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api');
  return { ...actual, apiFetch: apiFetchMock };
});

import { NextResponse } from 'next/server';

import { ApiError } from '@/lib/api';
import { apiErrorToResponse, proxyAPI, withValidatedBody } from '@/lib/api-route';
import { z } from 'zod';

afterEach(() => {
  apiFetchMock.mockReset();
});

describe('proxyAPI', () => {
  it('preserves rate-limit timing without forwarding authorization material', async () => {
    apiFetchMock.mockResolvedValue(
      new Response(
        JSON.stringify({ error: { code: 'RATE_LIMITED', message: 'too many requests' } }),
        {
          status: 429,
          headers: {
            'Content-Type': 'application/json',
            'Retry-After': '7',
            'X-RateLimit-Limit': '10',
            Authorization: 'Bearer must-not-leak',
          },
        },
      ),
    );

    const response = await proxyAPI('/api/v1/sites', { method: 'GET' }, 'fallback');

    expect(response.status).toBe(429);
    expect(response.headers.get('Retry-After')).toBe('7');
    expect(response.headers.get('X-RateLimit-Limit')).toBe('10');
    expect(response.headers.get('Authorization')).toBeNull();
    await expect(response.json()).resolves.toEqual({
      error: { code: 'RATE_LIMITED', message: 'too many requests' },
    });
  });

  it('maps a missing server session to enumeration-safe 401 JSON', async () => {
    apiFetchMock.mockRejectedValue(new ApiError('UNAUTHENTICATED', 401, 'unauthorized'));

    const response = await proxyAPI('/api/v1/sites', { method: 'GET' }, 'fallback');

    expect(response.status).toBe(401);
    await expect(response.json()).resolves.toEqual({
      error: { code: 'UNAUTHENTICATED', message: 'Sign in required' },
    });
  });

  it('preserves upstream validation status, code, and details', async () => {
    apiFetchMock.mockRejectedValue(
      new ApiError('Invalid hostname', 400, 'validation', [
        { field: 'hostname', issue: 'must be FQDN' },
      ]),
    );

    const response = await proxyAPI('/api/v1/domains', { method: 'POST' }, 'fallback');

    expect(response.status).toBe(400);
    await expect(response.json()).resolves.toEqual({
      error: {
        code: 'validation',
        message: 'Invalid hostname',
        details: [{ field: 'hostname', issue: 'must be FQDN' }],
      },
    });
  });

  it('falls back to 502 INTERNAL for non-ApiError exceptions', async () => {
    apiFetchMock.mockRejectedValue(new TypeError('network down'));

    const response = await proxyAPI('/api/v1/sites', { method: 'GET' }, 'fallback');

    expect(response.status).toBe(502);
    await expect(response.json()).resolves.toEqual({
      error: { code: 'INTERNAL', message: 'fallback' },
    });
  });
});

describe('apiErrorToResponse', () => {
  it('maps 401 to UNAUTHENTICATED regardless of upstream code', () => {
    const response = apiErrorToResponse(new ApiError('expired', 401, 'token_expired'));
    expect(response.status).toBe(401);
  });

  it('preserves details on validation errors', async () => {
    const response = apiErrorToResponse(
      new ApiError('bad input', 422, 'validation', [{ field: 'name', issue: 'too short' }]),
    );
    expect(response.status).toBe(422);
    const body = await response.json();
    expect(body.error.details).toEqual([{ field: 'name', issue: 'too short' }]);
  });
});

describe('withValidatedBody', () => {
  const schema = z.object({ name: z.string().min(1), count: z.number().int().positive() });

  it('returns 422 VALIDATION_FAILED on invalid JSON', async () => {
    const request = new Request('http://localhost/api/test', {
      method: 'POST',
      body: 'not json',
      headers: { 'Content-Type': 'application/json' },
    });

    const response = await withValidatedBody(request, schema, async () =>
      NextResponse.json({ ok: true }),
    );

    expect(response.status).toBe(422);
    const body = await response.json();
    expect(body.error.code).toBe('VALIDATION_FAILED');
    expect(body.error.message).toBe('Invalid JSON body');
  });

  it('returns 422 with field details on schema failure', async () => {
    const request = new Request('http://localhost/api/test', {
      method: 'POST',
      body: JSON.stringify({ name: '', count: -1 }),
      headers: { 'Content-Type': 'application/json' },
    });

    const response = await withValidatedBody(
      request,
      schema,
      async () => NextResponse.json({ ok: true }),
      { message: 'Fix the fields.' },
    );

    expect(response.status).toBe(422);
    const body = await response.json();
    expect(body.error.code).toBe('VALIDATION_FAILED');
    expect(body.error.message).toBe('Fix the fields.');
    expect(body.error.details).toHaveLength(2);
    expect(body.error.details.map((d: { field: string }) => d.field)).toEqual(['name', 'count']);
  });

  it('calls the handler with typed parsed data on success', async () => {
    const request = new Request('http://localhost/api/test', {
      method: 'POST',
      body: JSON.stringify({ name: 'widget', count: 5 }),
      headers: { 'Content-Type': 'application/json' },
    });

    const response = await withValidatedBody(request, schema, async (body) => {
      expect(body.name).toBe('widget');
      expect(body.count).toBe(5);
      return NextResponse.json({ ok: true });
    });

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({ ok: true });
  });

  it('translates ApiError thrown by the handler via apiErrorToResponse', async () => {
    const request = new Request('http://localhost/api/test', {
      method: 'POST',
      body: JSON.stringify({ name: 'widget', count: 5 }),
      headers: { 'Content-Type': 'application/json' },
    });

    const response = await withValidatedBody(request, schema, async () => {
      throw new ApiError('conflict', 409, 'duplicate');
    });

    expect(response.status).toBe(409);
    const body = await response.json();
    expect(body.error.code).toBe('duplicate');
  });
});
