import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const { getTokenMock, headersMock } = vi.hoisted(() => ({
  getTokenMock: vi.fn(),
  headersMock: vi.fn(),
}));

vi.mock('next/headers', () => ({ headers: headersMock }));
vi.mock('@/lib/auth', () => ({
  auth: { api: { getToken: getTokenMock } },
}));

import { ApiError, apiFetch, apiJSON } from '@/lib/api';

afterEach(() => {
  vi.restoreAllMocks();
  getTokenMock.mockReset();
  headersMock.mockReset();
  delete process.env.API_URL;
});

describe('apiFetch', () => {
  beforeEach(() => {
    process.env.API_URL = 'http://127.0.0.1:8080';
  });

  it('attaches JWT and sets no-store cache on success', async () => {
    const requestHeaders = new Headers({ cookie: 'session=opaque' });
    headersMock.mockResolvedValue(requestHeaders);
    getTokenMock.mockResolvedValue({ token: 'server-jwt' });

    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ data: { ok: true } }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    );

    const response = await apiFetch('/api/v1/sites');

    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('http://127.0.0.1:8080/api/v1/sites');
    expect(new Headers(init?.headers).get('Authorization')).toBe('Bearer server-jwt');
    expect(init?.cache).toBe('no-store');
    expect(await response.json()).toEqual({ data: { ok: true } });
  });

  it('throws ApiError when session has no JWT', async () => {
    headersMock.mockResolvedValue(new Headers());
    getTokenMock.mockResolvedValue(null);

    const fetchMock = vi.spyOn(globalThis, 'fetch');

    await expect(apiFetch('/api/v1/sites')).rejects.toThrow(ApiError);
    await expect(apiFetch('/api/v1/sites')).rejects.toMatchObject({
      name: 'ApiError',
      message: 'UNAUTHENTICATED',
      status: 401,
      code: 'unauthorized',
    });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('throws ApiError with details on backend 4xx errors', async () => {
    getTokenMock.mockResolvedValue({ token: 'jwt' });

    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            error: {
              code: 'validation',
              message: 'Invalid hostname',
              details: [{ field: 'hostname', issue: 'must be FQDN' }],
            },
          }),
          { status: 400, headers: { 'Content-Type': 'application/json' } },
        ),
      ),
    );

    await expect(apiFetch('/api/v1/domains')).rejects.toThrow(ApiError);

    try {
      await apiFetch('/api/v1/domains');
    } catch (error) {
      expect(error).toBeInstanceOf(ApiError);
      if (error instanceof ApiError) {
        expect(error.status).toBe(400);
        expect(error.code).toBe('validation');
        expect(error.message).toBe('Invalid hostname');
        expect(error.details).toHaveLength(1);
        expect(error.details?.[0]).toEqual({ field: 'hostname', issue: 'must be FQDN' });
      }
    }
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('strips Authorization from client-facing responses', async () => {
    getTokenMock.mockResolvedValue({ token: 'jwt' });

    const responseHeaders = new Headers({ 'Content-Type': 'application/json' });
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ data: {} }), {
        status: 200,
        headers: responseHeaders,
      }),
    );

    const response = await apiFetch('/api/v1/sites');
    expect(response.headers.get('Authorization')).toBeNull();
  });
});

describe('apiJSON', () => {
  it('returns parsed JSON body on success', async () => {
    const mockData = { sites: [{ id: 'site-1' }] };
    const headersMockInstance = vi.fn().mockResolvedValue(new Headers({ cookie: 'session=test' }));
    vi.mocked(headersMock).mockImplementation(headersMockInstance);
    getTokenMock.mockResolvedValue({ token: 'jwt' });

    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify(mockData), { status: 200, headers: { 'Content-Type': 'application/json' } }),
    );

    const result = await apiJSON<{ sites: unknown[] }>('/api/v1/sites');
    expect(result).toEqual(mockData);
  });

  it('throws typed ApiError instead of generic Error on failure', async () => {
    const headersMockInstance = vi.fn().mockResolvedValue(new Headers());
    vi.mocked(headersMock).mockImplementation(headersMockInstance);
    getTokenMock.mockResolvedValue({ token: 'jwt' });

    vi.spyOn(globalThis, 'fetch').mockImplementation(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({ error: { code: 'not_found', message: 'Site missing' } }),
          { status: 404, headers: { 'Content-Type': 'application/json' } },
        ),
      ),
    );

    await expect(apiJSON('/api/v1/sites/missing')).rejects.toThrow(ApiError);
    await expect(apiJSON('/api/v1/sites/missing')).rejects.toMatchObject({
      code: 'not_found',
      status: 404,
    });
  });
});
