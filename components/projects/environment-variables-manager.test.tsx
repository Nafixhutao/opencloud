import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { EnvironmentVariablesManager } from '@/components/projects/environment-variables-manager';
import type { EnvironmentVariable } from '@/lib/environment-variables';

const service = { id: '11111111-1111-1111-1111-111111111111', name: 'frontend' };

const plainVariable: EnvironmentVariable = {
  id: '22222222-2222-2222-2222-222222222222',
  key: 'NODE_ENV',
  value: 'production',
  is_secret: false,
  environment: 'production',
  created_at: '2026-08-09T00:00:00Z',
  updated_at: '2026-08-09T00:00:00Z',
};

const secretVariable: EnvironmentVariable = {
  id: '33333333-3333-3333-3333-333333333333',
  key: 'DATABASE_URL',
  is_secret: true,
  environment: 'production',
  created_at: '2026-08-09T00:00:00Z',
  updated_at: '2026-08-09T00:00:00Z',
};

const jsonResponse = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });

function renderManager(services = [service]) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <EnvironmentVariablesManager projectId="p1" services={services} />
    </QueryClientProvider>,
  );
}

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

describe('EnvironmentVariablesManager', () => {
  it('shows the empty state when the service has no variables', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(jsonResponse({ data: [] }));
    renderManager();
    expect(await screen.findByText('No variables in production')).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith(
      `/api/projects/p1/services/${service.id}/environment?environment=production`,
      expect.objectContaining({ cache: 'no-store' }),
    );
  });

  it('shows the no-service state instead of fetching when the project has no services', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(jsonResponse({ data: [] }));
    renderManager([]);
    expect(await screen.findByText('No service to configure')).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('validates the key before sending a create request', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(jsonResponse({ data: [] }));
    renderManager();
    await screen.findByText('No variables in production');
    fetchMock.mockClear();

    fireEvent.click(screen.getByRole('button', { name: 'Add Variable' }));
    fireEvent.change(screen.getByLabelText('Key'), { target: { value: 'invalid-key' } });
    fireEvent.change(screen.getByLabelText('Value'), { target: { value: 'x' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save Variable' }));

    expect(await screen.findByText(/Keys start with an uppercase letter/)).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('creates a variable and shows it after refetch', async () => {
    let created = false;
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async (_input, init) => {
      const method = (init?.method ?? 'GET').toUpperCase();
      if (method === 'POST') {
        created = true;
        return jsonResponse({ data: plainVariable }, 201);
      }
      return jsonResponse({ data: created ? [plainVariable] : [] });
    });
    renderManager();
    await screen.findByText('No variables in production');

    fireEvent.click(screen.getByRole('button', { name: 'Add Variable' }));
    fireEvent.change(screen.getByLabelText('Key'), { target: { value: 'node_env' } });
    fireEvent.change(screen.getByLabelText('Value'), { target: { value: 'production' } });
    fireEvent.click(screen.getByRole('button', { name: 'Save Variable' }));

    await waitFor(() => expect(screen.getByText('NODE_ENV')).toBeInTheDocument());
    const postCall = fetchMock.mock.calls.find(([, init]) => (init as RequestInit)?.method === 'POST');
    expect(postCall?.[1]).toMatchObject({
      method: 'POST',
      body: JSON.stringify({ key: 'NODE_ENV', value: 'production', is_secret: false, environment: 'production' }),
    });
  });

  it('reveals a secret only through the audited reveal endpoint', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      const url = String(input);
      const method = (init?.method ?? 'GET').toUpperCase();
      if (method === 'POST' && url.endsWith('/reveal')) {
        return jsonResponse({ data: { value: 'postgres://hunter2' } });
      }
      return jsonResponse({ data: [secretVariable] });
    });
    renderManager();
    expect(await screen.findByText('DATABASE_URL')).toBeInTheDocument();
    // Secrets never arrive with a value in list responses; only the Reveal control exists.
    expect(screen.queryByText('postgres://hunter2')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Reveal value of DATABASE_URL' }));

    expect(await screen.findByText('postgres://hunter2')).toBeInTheDocument();
    expect(fetchMock.mock.calls.some(([url, init]) =>
      String(url).endsWith(`/environment/${secretVariable.id}/reveal`) && (init as RequestInit)?.method === 'POST',
    )).toBe(true);
  });

  it('requires typing the key before a delete is confirmed', async () => {
    let deleted = false;
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      const method = (init?.method ?? 'GET').toUpperCase();
      if (method === 'DELETE') {
        deleted = true;
        return new Response(null, { status: 204 });
      }
      return jsonResponse({ data: deleted ? [] : [secretVariable] });
    });
    renderManager();
    expect(await screen.findByText('DATABASE_URL')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
    const confirm = screen.getByRole('button', { name: 'Delete Variable' });
    expect(confirm).toBeDisabled();

    fireEvent.change(screen.getByLabelText('Type the variable key to confirm deletion'), {
      target: { value: 'DATABASE_URL' },
    });
    expect(confirm).not.toBeDisabled();
    fireEvent.click(confirm);

    await waitFor(() =>
      expect(fetchMock.mock.calls.some(([url, init]) =>
        String(url).endsWith(`/environment/${secretVariable.id}`) && (init as RequestInit)?.method === 'DELETE',
      )).toBe(true),
    );
    await screen.findByText('No variables in production');
  });

  it('loads the audit trail only when activity is expanded', async () => {
    const auditEntry = {
      id: '44444444-4444-4444-4444-444444444444',
      action: 'revealed' as const,
      key: 'DATABASE_URL',
      is_secret: true,
      environment: 'production',
      actor_id: 'actor-1',
      created_at: '2026-08-16T00:00:00Z',
    };
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      const url = String(input);
      if (url.includes('/environment/audit')) {
        return jsonResponse({ data: [auditEntry] });
      }
      return jsonResponse({ data: [secretVariable] });
    });
    renderManager();
    expect(await screen.findByText('DATABASE_URL')).toBeInTheDocument();
    // Collapsed by default: the audit endpoint is untouched.
    expect(fetchMock.mock.calls.some(([url]) => String(url).includes('/environment/audit'))).toBe(false);

    fireEvent.click(screen.getByRole('button', { name: 'Show' }));

    expect(await screen.findByText('revealed')).toBeInTheDocument();
    expect(fetchMock.mock.calls.some(([url]) => String(url).includes('/environment/audit?limit=20'))).toBe(true);
    // Values never appear in the trail — only the key and action.
    expect(screen.queryByText('postgres://hunter2')).not.toBeInTheDocument();
  });
});
