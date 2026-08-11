import { beforeEach, describe, expect, it, vi } from 'vitest';

const { proxyAPIMock } = vi.hoisted(() => ({
  proxyAPIMock: vi.fn(),
}));

vi.mock('@/lib/api-route', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api-route')>('@/lib/api-route');
  return { ...actual, proxyAPI: proxyAPIMock };
});

import { DELETE as detachDomain, GET as getDomain } from '@/app/api/domains/[id]/route';
import { POST as rotateChallenge } from '@/app/api/domains/[id]/challenge/route';
import { GET as getInstructions } from '@/app/api/domains/[id]/instructions/route';
import { POST as verifyDomain } from '@/app/api/domains/[id]/verify/route';
import { GET as listDomains, POST as attachDomain } from '@/app/api/sites/[id]/domains/route';

const context = { params: Promise.resolve({ id: 'domain-or-site-id' }) };

beforeEach(() => {
  proxyAPIMock.mockReset();
  proxyAPIMock.mockResolvedValue(new Response(null, { status: 200 }));
});

describe('domain BFF routes', () => {
  it('forwards a tenant-authenticated site domain list', async () => {
    await listDomains(new Request('https://dashboard.example.test/api/sites/site/domains'), context);

    expect(proxyAPIMock).toHaveBeenCalledWith(
      '/api/v1/sites/domain-or-site-id/domains',
      { method: 'GET' },
      'Could not load domains.',
    );
  });

  it('normalizes and forwards bounded domain pagination', async () => {
    await listDomains(
      new Request(
        'https://dashboard.example.test/api/sites/site/domains?page=2&per_page=500',
      ),
      context,
    );

    expect(proxyAPIMock).toHaveBeenCalledWith(
      '/api/v1/sites/domain-or-site-id/domains?page=2&per_page=100',
      { method: 'GET' },
      'Could not load domains.',
    );
  });

  it('rejects an invalid hostname before calling the control plane', async () => {
    const response = await attachDomain(
      new Request('https://dashboard.example.test/api/sites/site/domains', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ hostname: 'https://example.com/path' }),
      }),
      context,
    );

    expect(response.status).toBe(422);
    expect(proxyAPIMock).not.toHaveBeenCalled();
  });

  it('forwards a validated hostname and stable idempotency key', async () => {
    await attachDomain(
      new Request('https://dashboard.example.test/api/sites/site/domains', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Idempotency-Key': 'domain-attempt-1',
        },
        body: JSON.stringify({ hostname: 'www.example.com' }),
      }),
      context,
    );

    expect(proxyAPIMock).toHaveBeenCalledWith(
      '/api/v1/sites/domain-or-site-id/domains',
      {
        method: 'POST',
        headers: { 'Idempotency-Key': 'domain-attempt-1' },
        body: JSON.stringify({ hostname: 'www.example.com' }),
      },
      'Could not attach the domain.',
    );
  });

  it('maps detail, instructions, verify, rotate, and detach to purpose-specific API routes', async () => {
    const request = new Request('https://dashboard.example.test/api/domains/domain');
    await getDomain(request, context);
    await getInstructions(request, context);
    await verifyDomain(request, context);
    await rotateChallenge(request, context);
    await detachDomain(request, context);

    expect(proxyAPIMock.mock.calls).toEqual([
      [
        '/api/v1/domains/domain-or-site-id',
        { method: 'GET' },
        'Could not load domain.',
      ],
      [
        '/api/v1/domains/domain-or-site-id/instructions',
        { method: 'GET' },
        'Could not load DNS instructions.',
      ],
      [
        '/api/v1/domains/domain-or-site-id/verify',
        { method: 'POST' },
        'Could not queue domain verification.',
      ],
      [
        '/api/v1/domains/domain-or-site-id/challenge',
        { method: 'POST' },
        'Could not rotate the ownership challenge.',
      ],
      [
        '/api/v1/domains/domain-or-site-id',
        { method: 'DELETE' },
        'Could not queue domain detachment.',
      ],
    ]);
  });

  it('forces raw ownership instructions to remain non-cacheable', async () => {
    const response = await getInstructions(
      new Request('https://dashboard.example.test/api/domains/domain/instructions'),
      context,
    );

    expect(response.headers.get('Cache-Control')).toBe('private, no-store');
    expect(response.headers.get('Pragma')).toBe('no-cache');
  });
});
