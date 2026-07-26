import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { SiteDashboard } from '@/components/sites/site-dashboard';
import type { Site, SitesEnvelope } from '@/lib/sites';

const emptySites: SitesEnvelope = {
  data: [],
  meta: { page: 1, per_page: 25, total: 0 },
};

function renderDashboard(initialData: SitesEnvelope = emptySites) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
      mutations: { retry: false },
    },
  });
  return render(
    <QueryClientProvider client={client}>
      <SiteDashboard initialData={initialData} />
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe('SiteDashboard', () => {
  it('validates a domain before sending a create request', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch');
    renderDashboard();

    fireEvent.change(screen.getByLabelText('Domain'), { target: { value: 'not a domain' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create site' }));

    expect(
      await screen.findByText('Enter a valid ASCII domain, for example site.example.com.'),
    ).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('shows a queued site after the API accepts creation', async () => {
    const site: Site = {
      id: '2b8b66f1-1a25-4f7f-8bb3-c4c54feaf4a1',
      domain: 'site.example.test',
      status: 'provisioning',
      created_at: '2026-07-26T00:00:00Z',
      updated_at: '2026-07-26T00:00:00Z',
    };
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ data: site }), {
          status: 202,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
      .mockResolvedValue(
        new Response(
          JSON.stringify({ data: [site], meta: { page: 1, per_page: 25, total: 1 } }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
      );
    renderDashboard();

    fireEvent.change(screen.getByLabelText('Domain'), {
      target: { value: 'site.example.test' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Create site' }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    expect(await screen.findAllByText('site.example.test')).not.toHaveLength(0);
    expect(screen.getAllByText('Provisioning')).not.toHaveLength(0);
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({ method: 'POST' });
  });
});
