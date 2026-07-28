import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { DatabaseDashboard } from '@/components/databases/database-dashboard';
import type {
  DatabaseCredentials,
  DatabasesEnvelope,
  ManagedDatabase,
} from '@/lib/databases';

const emptyDatabases: DatabasesEnvelope = {
  data: [],
  meta: { page: 1, per_page: 25, total: 0 },
};

function renderDashboard(initialData: DatabasesEnvelope = emptyDatabases) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Number.POSITIVE_INFINITY },
      mutations: { retry: false },
    },
  });
  return render(
    <QueryClientProvider client={client}>
      <DatabaseDashboard initialData={initialData} />
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe('DatabaseDashboard', () => {
  it('validates a managed database name before sending a request', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch');
    renderDashboard();

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: '9 invalid' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create database' }));

    expect(
      await screen.findByText(
        'Start with a letter and use only lowercase letters, numbers, underscores, or hyphens.',
      ),
    ).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('queues the selected engine and renders the durable pending state', async () => {
    const database = managedDatabase({ status: 'provisioning', engine: 'mariadb' });
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(jsonResponse({ data: database }, 202))
      .mockResolvedValue(
        jsonResponse({
          data: [database],
          meta: { page: 1, per_page: 25, total: 1 },
        }),
      );
    renderDashboard();

    fireEvent.change(screen.getByLabelText('Name'), { target: { value: 'orders' } });
    fireEvent.change(screen.getByLabelText('Engine'), { target: { value: 'mariadb' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create database' }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    expect(await screen.findAllByText('orders')).not.toHaveLength(0);
    expect(screen.getAllByText('Provisioning')).not.toHaveLength(0);
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({
      method: 'POST',
      body: JSON.stringify({ name: 'orders', engine: 'mariadb' }),
    });
  });

  it('requires confirmation, reveals credentials once, and marks them unavailable', async () => {
    const database = managedDatabase({
      status: 'active',
      credential_available: true,
    });
    const credentials: DatabaseCredentials = {
      engine: 'postgres',
      host: 'postgres.example.test',
      port: 5432,
      database: 'ocdb_0123456789abcdef0123456789abcdef',
      username: 'ocu_0123456789abcdef0123456789abcdef',
      password: 'one-time-password',
      tls_required: true,
    };
    const afterReveal = { ...database, credential_available: false };
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(jsonResponse({ data: credentials }))
      .mockResolvedValue(
        jsonResponse({
          data: [afterReveal],
          meta: { page: 1, per_page: 25, total: 1 },
        }),
      );
    renderDashboard({
      data: [database],
      meta: { page: 1, per_page: 25, total: 1 },
    });

    fireEvent.click(
      within(screen.getByRole('table')).getByRole('button', {
        name: 'Reveal once',
      }),
    );
    expect(
      await screen.findByText('Reveal credentials for orders?'),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Reveal credentials' }));

    expect(await screen.findByText('one-time-password')).toBeInTheDocument();
    expect(screen.getByText('Save credentials for orders')).toBeInTheDocument();
    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      `/api/databases/${database.id}/credentials/reveal`,
    );
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({ method: 'POST' });
    await waitFor(() =>
      expect(screen.queryByRole('button', { name: 'Reveal once' })).not.toBeInTheDocument(),
    );
  });

  it('requires the exact name before queueing deletion', async () => {
    const database = managedDatabase({
      status: 'active',
      credential_available: false,
    });
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(
        jsonResponse({ data: { ...database, status: 'deleting' } }, 202),
      )
      .mockResolvedValue(
        jsonResponse({
          data: [],
          meta: { page: 1, per_page: 25, total: 0 },
        }),
      );
    renderDashboard({
      data: [database],
      meta: { page: 1, per_page: 25, total: 1 },
    });

    fireEvent.click(
      within(screen.getByRole('table')).getByRole('button', { name: 'Delete' }),
    );
    const deleteButton = await screen.findByRole('button', { name: 'Delete database' });
    expect(deleteButton).toBeDisabled();
    fireEvent.change(screen.getByLabelText('Database name'), {
      target: { value: 'orders' },
    });
    expect(deleteButton).toBeEnabled();
    fireEvent.click(deleteButton);

    await waitFor(() =>
      expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({ method: 'DELETE' }),
    );
  });

  it('loads and deletes a database beyond the first 25, then returns to the last valid page', async () => {
    const firstPage = Array.from({ length: 25 }, (_, index) =>
      managedDatabase({
        id: `00000000-0000-0000-0000-${String(index + 1).padStart(12, '0')}`,
        name: `database_${index + 1}`,
      }),
    );
    const database26 = managedDatabase({
      id: '00000000-0000-0000-0000-000000000026',
      name: 'database_26',
    });
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(
        jsonResponse({
          data: [database26],
          meta: { page: 2, per_page: 25, total: 26 },
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse({ data: { ...database26, status: 'deleting' } }, 202),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          data: [],
          meta: { page: 2, per_page: 25, total: 25 },
        }),
      )
      .mockResolvedValueOnce(
        jsonResponse({
          data: firstPage,
          meta: { page: 1, per_page: 25, total: 25 },
        }),
      );
    renderDashboard({
      data: firstPage,
      meta: { page: 1, per_page: 25, total: 26 },
    });

    fireEvent.click(screen.getByRole('button', { name: 'Next' }));
    expect(await screen.findAllByText('database_26')).not.toHaveLength(0);
    expect(fetchMock.mock.calls[0]?.[0]).toBe(
      '/api/databases?page=2&per_page=25',
    );

    fireEvent.click(
      within(screen.getByRole('table')).getByRole('button', { name: 'Delete' }),
    );
    fireEvent.change(await screen.findByLabelText('Database name'), {
      target: { value: 'database_26' },
    });
    fireEvent.click(screen.getByRole('button', { name: 'Delete database' }));

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        `/api/databases/${database26.id}`,
        expect.objectContaining({ method: 'DELETE' }),
      ),
    );
    await waitFor(() =>
      expect(fetchMock.mock.calls.some(
        ([path]) => path === '/api/databases?page=1&per_page=25',
      )).toBe(true),
    );
  });
});

function managedDatabase(
  overrides: Partial<ManagedDatabase> = {},
): ManagedDatabase {
  return {
    id: 'a4f4d8ea-f7b4-42ca-9081-955eb77a75ca',
    name: 'orders',
    engine: 'postgres',
    status: 'active',
    credential_available: false,
    created_at: '2026-07-27T00:00:00Z',
    updated_at: '2026-07-27T00:00:00Z',
    ...overrides,
  };
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}
