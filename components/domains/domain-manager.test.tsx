import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

const { routerPushMock, routerReplaceMock, searchParamsMock } = vi.hoisted(() => ({
  routerPushMock: vi.fn(),
  routerReplaceMock: vi.fn(),
  searchParamsMock: new URLSearchParams(),
}));

vi.mock('next/navigation', () => ({
  usePathname: () => `/sites/${site.id}`,
  useRouter: () => ({ push: routerPushMock, replace: routerReplaceMock }),
  useSearchParams: () => searchParamsMock,
}));

import { DomainManager } from '@/components/domains/domain-manager';
import type { Domain, DomainsEnvelope } from '@/lib/domains';
import type { Site } from '@/lib/sites';

const site: Site = {
  id: '11111111-1111-4111-8111-111111111111',
  domain: 'primary.example.com',
  status: 'active',
  created_at: '2026-07-30T00:00:00Z',
  updated_at: '2026-07-30T00:00:00Z',
};

const pendingDomain: Domain = {
  id: '22222222-2222-4222-8222-222222222222',
  site_id: site.id,
  hostname: 'www.example.com',
  status: 'pending',
  verification_expires_at: '2026-07-30T08:00:00Z',
  dns_provider: 'manual',
  cert_status: 'none',
  cert_auto_renew: true,
  created_at: '2026-07-30T07:00:00Z',
  updated_at: '2026-07-30T07:00:00Z',
};

function envelope(domains: Domain[]): DomainsEnvelope {
  return { data: domains, meta: { page: 1, per_page: 25, total: domains.length } };
}

function pageEnvelope(
  domains: Domain[],
  page: number,
  total: number,
): DomainsEnvelope {
  return { data: domains, meta: { page, per_page: 25, total } };
}

function jsonResponse(body: unknown, status = 200, headers?: HeadersInit) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json', ...headers },
  });
}

function instructions(domain = pendingDomain) {
  const records = domain.verified_at
    ? [{ type: 'A', name: domain.hostname, content: '203.0.113.10', ttl: 300 }]
    : [{
        type: 'TXT',
        name: `_opencloud-verification.${domain.hostname}`,
        content: 'ownership-token',
        ttl: 300,
      }];
  return {
    data: {
      verification_expires_at: domain.verification_expires_at,
      records,
    },
  };
}

function renderDashboard(initialData: DomainsEnvelope, siteOverride: Site = site) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
      mutations: { retry: false },
    },
  });
  const rendered = render(
    <QueryClientProvider client={client}>
      <DomainManager site={siteOverride} initialData={initialData} />
    </QueryClientProvider>,
  );
  return { ...rendered, client };
}

async function advancePollingTimers(milliseconds: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(milliseconds);
    await Promise.resolve();
    await vi.advanceTimersByTimeAsync(0);
  });
}

afterEach(() => {
  vi.useRealTimers();
  cleanup();
  vi.restoreAllMocks();
  routerPushMock.mockReset();
  routerReplaceMock.mockReset();
  searchParamsMock.delete('page');
});

describe('DomainManager', () => {
  it('validates then attaches a hostname and renders the real pending state', async () => {
    let current: Domain[] = [];
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      const path = String(input);
      if (path === `/api/sites/${site.id}/domains` && init?.method === 'POST') {
        current = [pendingDomain];
        return jsonResponse({ data: pendingDomain }, 201);
      }
      if (path === `/api/sites/${site.id}/domains`) {
        return jsonResponse(envelope(current));
      }
      if (path === `/api/domains/${pendingDomain.id}/instructions`) {
        return jsonResponse(instructions());
      }
      throw new Error(`Unexpected request: ${path}`);
    });
    renderDashboard(envelope([]));

    fireEvent.change(screen.getByLabelText('Hostname'), {
      target: { value: 'https://example.com/path' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Attach hostname' }));
    expect(
      await screen.findByText('Enter a hostname only, for example www.example.com.'),
    ).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText('Hostname'), {
      target: { value: pendingDomain.hostname },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Attach hostname' }));

    expect(await screen.findByText(pendingDomain.hostname)).toBeInTheDocument();
    expect(screen.getByText('DNS setup')).toBeInTheDocument();
    const attachCall = fetchMock.mock.calls.find(([, init]) => init?.method === 'POST');
    expect(attachCall?.[0]).toBe(`/api/sites/${site.id}/domains`);
    expect(attachCall?.[1]?.headers).toMatchObject({
      'Content-Type': 'application/json',
      'Idempotency-Key': expect.any(String),
    });
  });

  it('shows exact DNS instructions and moves to checking state after verify', async () => {
    const verifying = { ...pendingDomain, status: 'verifying' as const };
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(window.navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    });
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      const path = String(input);
      if (path.endsWith('/instructions')) {
        return jsonResponse(instructions());
      }
      if (path.endsWith('/verify') && init?.method === 'POST') {
        return jsonResponse({ data: verifying }, 202);
      }
      if (path === `/api/sites/${site.id}/domains`) {
        return jsonResponse(envelope([verifying]));
      }
      throw new Error(`Unexpected request: ${path}`);
    });
    renderDashboard(envelope([pendingDomain]));

    fireEvent.click(screen.getByRole('button', { name: 'Show DNS records' }));
    expect(await screen.findByText('ownership-token')).toBeInTheDocument();
    expect(
      screen.getByText(`_opencloud-verification.${pendingDomain.hostname}`),
    ).toBeInTheDocument();
    expect(screen.queryByText('203.0.113.10')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Copy TXT record value' }));
    expect(await screen.findByRole('button', { name: 'TXT record value copied' })).toBeInTheDocument();
    expect(writeText).toHaveBeenCalledWith('ownership-token');
    fireEvent.click(screen.getByRole('button', { name: 'Check DNS' }));

    expect(await screen.findByText('Checking DNS')).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith(
      `/api/domains/${pendingDomain.id}/verify`,
      expect.objectContaining({ method: 'POST' }),
    );
  });

  it('reuses an attach idempotency key until the normalized hostname changes', async () => {
    const changedDomain: Domain = {
      ...pendingDomain,
      id: '33333333-3333-4333-8333-333333333333',
      hostname: 'api.example.com',
    };
    const keys: string[] = [];
    let attempts = 0;
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      const path = String(input);
      if (path === `/api/sites/${site.id}/domains` && init?.method === 'POST') {
        keys.push(new Headers(init.headers).get('Idempotency-Key') ?? '');
        attempts++;
        if (attempts <= 2) {
          return jsonResponse(
            { error: { code: 'UNAVAILABLE', message: 'The attach result is not available yet.' } },
            503,
          );
        }
        return jsonResponse({ data: changedDomain }, 201);
      }
      if (path === `/api/sites/${site.id}/domains`) {
        return jsonResponse(envelope([changedDomain]));
      }
      if (path === `/api/domains/${changedDomain.id}/instructions`) {
        return jsonResponse(instructions(changedDomain));
      }
      throw new Error(`Unexpected request: ${path}`);
    });
    renderDashboard(envelope([]));

    const hostname = screen.getByLabelText('Hostname');
    fireEvent.change(hostname, { target: { value: `  ${pendingDomain.hostname.toUpperCase()}  ` } });
    fireEvent.click(screen.getByRole('button', { name: 'Attach hostname' }));
    expect(await screen.findByText('The attach result is not available yet.')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Attach hostname' }));
    await waitFor(() => expect(keys).toHaveLength(2));

    fireEvent.change(hostname, { target: { value: changedDomain.hostname } });
    fireEvent.click(screen.getByRole('button', { name: 'Attach hostname' }));
    expect(await screen.findByText(changedDomain.hostname)).toBeInTheDocument();
    expect(keys).toHaveLength(3);
    expect(keys[0]).not.toBe('');
    expect(keys[1]).toBe(keys[0]);
    expect(keys[2]).not.toBe(keys[0]);
  });

  it('shows a control-plane error when retry cannot be queued', async () => {
    const failed = {
      ...pendingDomain,
      status: 'failed' as const,
      last_error: 'Public DNS did not return the expected ownership value.',
    };
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      const path = String(input);
      if (path.endsWith('/instructions')) {
        return jsonResponse(instructions(failed));
      }
      if (path.endsWith('/verify') && init?.method === 'POST') {
        return jsonResponse(
          { error: { code: 'UNAVAILABLE', message: 'Verification is temporarily unavailable.' } },
          503,
        );
      }
      throw new Error(`Unexpected request: ${path}`);
    });
    renderDashboard(envelope([failed]));

    fireEvent.click(screen.getByRole('button', { name: 'Retry setup' }));
    expect(await screen.findByText('Verification is temporarily unavailable.')).toBeInTheDocument();
    expect(screen.getByText('Needs attention')).toBeInTheDocument();
  });

  it('does not fan out DNS instruction requests for a full page', async () => {
    const domains = Array.from({ length: 25 }, (_, index): Domain => ({
      ...pendingDomain,
      id: `${String(index + 1).padStart(8, '0')}-2222-4222-8222-222222222222`,
      hostname: `customer-${index + 1}.example.com`,
    }));
    let instructionRequests = 0;
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      const path = String(input);
      if (path.endsWith('/instructions')) {
        instructionRequests++;
        const domain = domains.find((candidate) => path.includes(candidate.id));
        return jsonResponse(instructions(domain));
      }
      throw new Error(`Unexpected request: ${path}`);
    });
    renderDashboard(envelope(domains));

    await Promise.resolve();
    expect(instructionRequests).toBe(0);
    fireEvent.click(screen.getAllByRole('button', { name: 'Show DNS records' })[0]);
    expect(await screen.findByText('ownership-token')).toBeInTheDocument();
    expect(instructionRequests).toBe(1);
  });

  it('maps backend hostname details to the hostname field', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      jsonResponse(
        {
          error: {
            code: 'VALIDATION',
            message: 'invalid hostname',
            details: [{ field: 'hostname', issue: 'uses an unrecognized public suffix' }],
          },
        },
        422,
      ),
    );
    renderDashboard(envelope([]));
    const hostname = screen.getByLabelText('Hostname');
    fireEvent.change(hostname, { target: { value: 'tenant.unknown' } });
    fireEvent.click(screen.getByRole('button', { name: 'Attach hostname' }));

    expect(
      await screen.findByText('uses an unrecognized public suffix'),
    ).toBeInTheDocument();
    expect(hostname).toHaveAttribute('aria-invalid', 'true');
    expect(hostname).toHaveFocus();
  });

  it('retries a row-scoped DNS instruction failure', async () => {
    let attempts = 0;
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      const path = String(input);
      if (path.endsWith('/instructions')) {
        attempts++;
        if (attempts === 1) {
          return jsonResponse(
            { error: { code: 'UNAVAILABLE', message: 'DNS records are temporarily unavailable.' } },
            503,
          );
        }
        return jsonResponse(instructions());
      }
      throw new Error(`Unexpected request: ${path}`);
    });
    renderDashboard(envelope([pendingDomain]));

    fireEvent.click(screen.getByRole('button', { name: 'Show DNS records' }));
    expect(
      await screen.findByText('DNS records are temporarily unavailable.'),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Retry DNS records' }));
    expect(await screen.findByText('ownership-token')).toBeInTheDocument();
    expect(attempts).toBe(2);
  });

  it('backs off and resumes polling after a transient status refresh failure', async () => {
    vi.useFakeTimers();
    const verifyingDomain: Domain = { ...pendingDomain, status: 'verifying' };
    let attempts = 0;
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      const path = String(input);
      if (path !== `/api/sites/${site.id}/domains`) {
        throw new Error(`Unexpected request: ${path}`);
      }
      attempts++;
      if (attempts === 1) {
        return jsonResponse(
          {
            error: {
              code: 'UNAVAILABLE',
              message: 'Domain status is temporarily unavailable.',
            },
          },
          503,
        );
      }
      return jsonResponse(envelope([verifyingDomain]));
    });
    renderDashboard(envelope([verifyingDomain]));

    await advancePollingTimers(2_000);
    expect(attempts).toBe(1);

    await advancePollingTimers(10_000);
    expect(attempts).toBe(2);

    await advancePollingTimers(2_000);
    expect(attempts).toBe(3);
  });

  it('stops automatic polling for a permanent list error', async () => {
    vi.useFakeTimers();
    const verifyingDomain: Domain = { ...pendingDomain, status: 'verifying' };
    let attempts = 0;
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      const path = String(input);
      if (path !== `/api/sites/${site.id}/domains`) {
        throw new Error(`Unexpected request: ${path}`);
      }
      attempts++;
      if (attempts === 1) {
        return jsonResponse(
          { error: { code: 'FORBIDDEN', message: 'Domain access is no longer allowed.' } },
          403,
        );
      }
      return jsonResponse(envelope([verifyingDomain]));
    });
    renderDashboard(envelope([verifyingDomain]));

    await advancePollingTimers(2_000);
    expect(attempts).toBe(1);

    await advancePollingTimers(20_000);
    expect(attempts).toBe(1);
  });

  it.each([
    {
      failure: () => Promise.resolve(jsonResponse(
        { error: { code: 'TIMEOUT', message: 'The control plane timed out.' } },
        408,
      )),
      label: 'an HTTP request timeout',
    },
    {
      failure: () => Promise.reject(new TypeError('network unavailable')),
      label: 'a network failure',
    },
  ])('backs off after $label while stale pending data remains', async ({ failure }) => {
    vi.useFakeTimers();
    const verifyingDomain: Domain = { ...pendingDomain, status: 'verifying' };
    let attempts = 0;
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      const path = String(input);
      if (path !== `/api/sites/${site.id}/domains`) {
        throw new Error(`Unexpected request: ${path}`);
      }
      attempts++;
      if (attempts === 1) {
        return failure();
      }
      return jsonResponse(envelope([verifyingDomain]));
    });
    renderDashboard(envelope([verifyingDomain]));

    await advancePollingTimers(2_000);
    expect(attempts).toBe(1);
    await advancePollingTimers(9_999);
    expect(attempts).toBe(1);
    await advancePollingTimers(1);
    expect(attempts).toBe(2);
  });

  it('honors Retry-After when list polling is rate limited', async () => {
    vi.useFakeTimers();
    const verifyingDomain: Domain = { ...pendingDomain, status: 'verifying' };
    let attempts = 0;
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      const path = String(input);
      if (path !== `/api/sites/${site.id}/domains`) {
        throw new Error(`Unexpected request: ${path}`);
      }
      attempts++;
      if (attempts === 1) {
        return jsonResponse(
          { error: { code: 'RATE_LIMITED', message: 'Slow down.' } },
          429,
          { 'Retry-After': '7' },
        );
      }
      return jsonResponse(envelope([verifyingDomain]));
    });
    renderDashboard(envelope([verifyingDomain]));

    await advancePollingTimers(2_000);
    expect(attempts).toBe(1);

    await advancePollingTimers(6_999);
    expect(attempts).toBe(1);
    await advancePollingTimers(1);
    expect(attempts).toBe(2);
  });

  it('stops list polling when the session expires', async () => {
    vi.useFakeTimers();
    const verifyingDomain: Domain = { ...pendingDomain, status: 'verifying' };
    let attempts = 0;
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      const path = String(input);
      if (path !== `/api/sites/${site.id}/domains`) {
        throw new Error(`Unexpected request: ${path}`);
      }
      attempts++;
      return jsonResponse(
        { error: { code: 'UNAUTHENTICATED', message: 'Sign in required.' } },
        401,
      );
    });
    renderDashboard(envelope([verifyingDomain]));

    await advancePollingTimers(2_000);
    expect(attempts).toBe(1);

    await advancePollingTimers(20_000);
    expect(attempts).toBe(1);
  });

  it('shows one accessible stale-data error and lets the user retry a permanent failure', async () => {
    const verifyingDomain: Domain = { ...pendingDomain, status: 'verifying' };
    let attempts = 0;
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      const path = String(input);
      if (path !== `/api/sites/${site.id}/domains`) {
        throw new Error(`Unexpected request: ${path}`);
      }
      attempts++;
      if (attempts === 1) {
        return jsonResponse(
          { error: { code: 'FORBIDDEN', message: 'Domain access is no longer allowed.' } },
          403,
        );
      }
      return jsonResponse(envelope([verifyingDomain]));
    });
    const rendered = renderDashboard(envelope([verifyingDomain]));

    await act(async () => {
      await rendered.client.refetchQueries({
        queryKey: ['domains', site.id, 1, 25],
        exact: true,
      });
    });
    expect(await screen.findByText('Domain access is no longer allowed.')).toBeInTheDocument();
    expect(screen.getAllByRole('alert')).toHaveLength(1);
    fireEvent.click(screen.getByRole('button', { name: 'Retry status' }));

    await waitFor(() => expect(attempts).toBe(2));
    await waitFor(() =>
      expect(screen.queryByText('Domain access is no longer allowed.')).not.toBeInTheDocument(),
    );
    expect(
      screen.getByText('Refreshing DNS, routing, and certificate status…'),
    ).toBeInTheDocument();
  });

  it('redirects a list refresh 401 with the safe current path', async () => {
    const verifyingDomain: Domain = { ...pendingDomain, status: 'verifying' };
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      jsonResponse(
        { error: { code: 'UNAUTHENTICATED', message: 'Sign in required.' } },
        401,
      ),
    );
    const rendered = renderDashboard(envelope([verifyingDomain]));

    await act(async () => {
      await rendered.client.refetchQueries({
        queryKey: ['domains', site.id, 1, 25],
        exact: true,
      });
    });
    await waitFor(() =>
      expect(routerReplaceMock).toHaveBeenCalledWith(
        `/login?notice=session-expired&next=${encodeURIComponent(`/sites/${site.id}`)}`,
      ),
    );
  });

  it('redirects mutation and instruction 401 responses with a safe return path', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      jsonResponse(
        { error: { code: 'UNAUTHENTICATED', message: 'Sign in required' } },
        401,
      ),
    );
    const firstRender = renderDashboard(envelope([]));
    fireEvent.change(screen.getByLabelText('Hostname'), {
      target: { value: pendingDomain.hostname },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Attach hostname' }));
    await waitFor(() =>
      expect(routerReplaceMock).toHaveBeenCalledWith(
        `/login?notice=session-expired&next=${encodeURIComponent(`/sites/${site.id}`)}`,
      ),
    );

    firstRender.unmount();
    routerReplaceMock.mockReset();
    renderDashboard(envelope([pendingDomain]));
    fireEvent.click(screen.getByRole('button', { name: 'Show DNS records' }));
    await waitFor(() =>
      expect(routerReplaceMock).toHaveBeenCalledWith(
        `/login?notice=session-expired&next=${encodeURIComponent(`/sites/${site.id}`)}`,
      ),
    );
  });

  it('announces clipboard failure and renders hydration-stable UTC dates', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      const path = String(input);
      if (path.endsWith('/instructions')) {
        return jsonResponse(instructions());
      }
      throw new Error(`Unexpected request: ${path}`);
    });
    Object.defineProperty(window.navigator, 'clipboard', {
      configurable: true,
      value: { writeText: vi.fn().mockRejectedValue(new Error('clipboard denied')) },
    });
    renderDashboard(envelope([pendingDomain]));

    expect(screen.getByText(/UTC$/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Show DNS records' }));
    await screen.findByText('ownership-token');
    fireEvent.click(screen.getByRole('button', { name: 'Copy TXT record value' }));
    expect(
      await screen.findByText('Copy failed. Select the value and copy it manually.'),
    ).toBeInTheDocument();
  });

  it('gives status-specific guidance when attachment is disabled', () => {
    for (const [status, guidance] of [
      ['provisioning', 'Wait for site provisioning to finish before attaching a hostname.'],
      ['suspended', 'Resume this site before attaching another hostname.'],
      ['failed', 'Delete this failed site and create a healthy site before attaching a hostname.'],
    ] as const) {
      const rendered = renderDashboard(envelope([]), { ...site, status });
      expect(screen.getByText(guidance)).toBeInTheDocument();
      rendered.unmount();
    }
  });

  it('requires hostname confirmation before detach and exposes certificate expiry', async () => {
    const active: Domain = {
      ...pendingDomain,
      status: 'active',
      verified_at: '2026-07-30T07:05:00Z',
      cert_status: 'active',
      cert_expires_at: '2026-10-28T07:05:00Z',
      cert_observed_at: '2026-07-30T07:10:00Z',
    };
    const deleting = { ...active, status: 'deleting' as const };
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      const path = String(input);
      if (path.endsWith('/instructions')) {
        return jsonResponse({
          data: {
            verification_expires_at: active.verification_expires_at,
            records: [{ type: 'A', name: active.hostname, content: '203.0.113.10', ttl: 300 }],
          },
        });
      }
      if (path === `/api/domains/${active.id}` && init?.method === 'DELETE') {
        return jsonResponse({ data: deleting }, 202);
      }
      if (path === `/api/sites/${site.id}/domains`) {
        return jsonResponse(envelope([deleting]));
      }
      throw new Error(`Unexpected request: ${path}`);
    });
    renderDashboard(envelope([active]));

    expect(screen.getByText(/Automatic renewal is on/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Detach' }));
    const dialog = await screen.findByRole('alertdialog');
    const confirm = within(dialog).getByLabelText('Hostname');
    const detachButton = within(dialog).getByRole('button', { name: 'Detach domain' });
    confirm.focus();
    expect(confirm).toHaveFocus();
    expect(detachButton).toBeDisabled();
    fireEvent.change(confirm, { target: { value: active.hostname } });
    expect(detachButton).toBeEnabled();
    fireEvent.click(detachButton);

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        `/api/domains/${active.id}`,
        expect.objectContaining({ method: 'DELETE' }),
      ),
    );
    expect(await screen.findByText('Detaching')).toBeInTheDocument();
  });

  it('keeps detach confirmation open and shows its row-scoped failure', async () => {
    const active: Domain = {
      ...pendingDomain,
      status: 'active',
      verified_at: '2026-07-30T07:05:00Z',
      cert_status: 'active',
    };
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      const path = String(input);
      if (path.endsWith('/instructions')) {
        return jsonResponse(instructions(active));
      }
      if (path === `/api/domains/${active.id}` && init?.method === 'DELETE') {
        return jsonResponse(
          { error: { code: 'UNAVAILABLE', message: 'Detachment is temporarily unavailable.' } },
          503,
        );
      }
      throw new Error(`Unexpected request: ${path}`);
    });
    renderDashboard(envelope([active]));

    fireEvent.click(screen.getByRole('button', { name: 'Detach' }));
    const dialog = await screen.findByRole('alertdialog');
    fireEvent.change(within(dialog).getByLabelText('Hostname'), {
      target: { value: active.hostname },
    });
    fireEvent.click(within(dialog).getByRole('button', { name: 'Detach domain' }));

    expect(
      await within(dialog).findByText('Detachment is temporarily unavailable.'),
    ).toBeInTheDocument();
    expect(dialog).toBeInTheDocument();
    expect(within(dialog).getByLabelText('Hostname')).toHaveValue(active.hostname);
  });

  it('lets URL navigation own later pages and restores server-provided page state', async () => {
    const laterDomain: Domain = {
      ...pendingDomain,
      id: '44444444-4444-4444-8444-444444444444',
      hostname: 'page-two.example.com',
      status: 'active',
      verified_at: '2026-07-30T07:05:00Z',
      cert_status: 'active',
    };
    const firstPage = pageEnvelope([pendingDomain], 1, 26);
    const secondPage = pageEnvelope([laterDomain], 2, 26);
    const fetchMock = vi.spyOn(globalThis, 'fetch');
    const firstRender = renderDashboard(firstPage);

    expect(screen.getByText('Page 1 of 2')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Next' }));
    expect(routerPushMock).toHaveBeenCalledWith(
      `/sites/${site.id}?page=2`,
      { scroll: false },
    );
    expect(fetchMock).not.toHaveBeenCalled();

    firstRender.unmount();
    searchParamsMock.set('page', '2');
    renderDashboard(secondPage);
    expect(screen.getByText(laterDomain.hostname)).toBeInTheDocument();
    expect(screen.getByText('Page 2 of 2')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Previous' }));
    expect(routerPushMock).toHaveBeenLastCalledWith(
      `/sites/${site.id}`,
      { scroll: false },
    );
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('redirects an out-of-range server page to the last valid URL', async () => {
    searchParamsMock.set('page', '3');
    renderDashboard(pageEnvelope([], 3, 26));

    await waitFor(() =>
      expect(routerReplaceMock).toHaveBeenCalledWith(
        `/sites/${site.id}?page=2`,
        { scroll: false },
      ),
    );
    expect(screen.queryByText('No custom domains')).not.toBeInTheDocument();
  });
});
