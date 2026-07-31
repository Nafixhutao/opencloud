import { afterEach, describe, expect, it, vi } from 'vitest';

const { apiFetchMock } = vi.hoisted(() => ({ apiFetchMock: vi.fn() }));

vi.mock('@/lib/api', () => ({ apiFetch: apiFetchMock }));

import { proxyAPI } from '@/lib/api-route';

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
    apiFetchMock.mockRejectedValue(new Error('UNAUTHENTICATED'));

    const response = await proxyAPI('/api/v1/sites', { method: 'GET' }, 'fallback');

    expect(response.status).toBe(401);
    await expect(response.json()).resolves.toEqual({
      error: { code: 'UNAUTHENTICATED', message: 'Sign in required' },
    });
  });
});
