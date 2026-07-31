import { afterEach, describe, expect, it, vi } from 'vitest';

const { getTokenMock, headersMock } = vi.hoisted(() => ({
  getTokenMock: vi.fn(),
  headersMock: vi.fn(),
}));

vi.mock('next/headers', () => ({ headers: headersMock }));
vi.mock('@/lib/auth', () => ({
  auth: { api: { getToken: getTokenMock } },
}));

import { apiFetch } from '@/lib/api';

afterEach(() => {
  vi.restoreAllMocks();
  getTokenMock.mockReset();
  headersMock.mockReset();
  delete process.env.API_URL;
});

describe('apiFetch', () => {
  it('attaches the server-side JWT without exposing it to the browser response', async () => {
    const requestHeaders = new Headers({ cookie: 'session=opaque' });
    headersMock.mockResolvedValue(requestHeaders);
    getTokenMock.mockResolvedValue({ token: 'server-only-jwt' });
    process.env.API_URL = 'http://api.internal:8080';
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ data: { ok: true } }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    );

    const response = await apiFetch('/api/v1/sites');

    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('http://api.internal:8080/api/v1/sites');
    expect(new Headers(init?.headers).get('Authorization')).toBe('Bearer server-only-jwt');
    expect(init?.cache).toBe('no-store');
    expect(await response.json()).toEqual({ data: { ok: true } });
    expect(response.headers.get('Authorization')).toBeNull();
  });

  it('fails before an upstream request when the session has no JWT', async () => {
    headersMock.mockResolvedValue(new Headers());
    getTokenMock.mockResolvedValue(null);
    const fetchMock = vi.spyOn(globalThis, 'fetch');

    await expect(apiFetch('/api/v1/sites')).rejects.toThrow('UNAUTHENTICATED');
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
