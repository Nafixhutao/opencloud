import { beforeEach, describe, expect, it, vi } from 'vitest';

const { proxyAPIMock } = vi.hoisted(() => ({
  proxyAPIMock: vi.fn(),
}));

vi.mock('@/lib/api-route', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api-route')>('@/lib/api-route');
  return { ...actual, proxyAPI: proxyAPIMock };
});

import { GET } from '@/app/api/databases/route';

beforeEach(() => {
  proxyAPIMock.mockReset();
  proxyAPIMock.mockResolvedValue(new Response(null, { status: 200 }));
});

describe('database BFF pagination', () => {
  it('forwards validated page and per_page values', async () => {
    await GET(
      new Request('https://dashboard.example.test/api/databases?page=2&per_page=50'),
    );

    expect(proxyAPIMock).toHaveBeenCalledWith(
      '/api/v1/databases?page=2&per_page=50',
      { method: 'GET' },
      'Could not load databases.',
    );
  });

  it('normalizes invalid values and caps per_page', async () => {
    await GET(
      new Request(
        'https://dashboard.example.test/api/databases?page=not-a-page&per_page=1000',
      ),
    );

    expect(proxyAPIMock).toHaveBeenCalledWith(
      '/api/v1/databases?page=1&per_page=100',
      { method: 'GET' },
      'Could not load databases.',
    );
  });
});
