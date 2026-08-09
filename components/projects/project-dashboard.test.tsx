import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { ProjectDashboard } from '@/components/projects/project-dashboard';
import type { ProjectsEnvelope } from '@/lib/projects';

const emptyProjects: ProjectsEnvelope = { data: [], meta: { page: 1, per_page: 25, total: 0 } };

function renderDashboard(initialData: ProjectsEnvelope = emptyProjects) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Number.POSITIVE_INFINITY }, mutations: { retry: false } } });
  return render(<QueryClientProvider client={client}><ProjectDashboard initialData={initialData} /></QueryClientProvider>);
}

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

describe('ProjectDashboard', () => {
  it('validates a project name before sending a create request', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch');
    renderDashboard();
    fireEvent.click(screen.getByRole('button', { name: 'Create project' }));
    expect(await screen.findByText('Enter a project name.')).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('renders the real project returned by the control plane', async () => {
    const project = { id: '2b8b66f1-1a25-4f7f-8bb3-c4c54feaf4a1', name: 'toko-online', status: 'active' as const, created_at: '2026-08-09T00:00:00Z', updated_at: '2026-08-09T00:00:00Z' };
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(new Response(JSON.stringify({ data: project }), { status: 201, headers: { 'Content-Type': 'application/json' } })).mockResolvedValue(new Response(JSON.stringify({ data: [project], meta: { page: 1, per_page: 25, total: 1 } }), { status: 200, headers: { 'Content-Type': 'application/json' } }));
    renderDashboard();
    fireEvent.change(screen.getByLabelText('Project name'), { target: { value: 'toko-online' } });
    fireEvent.click(screen.getByRole('button', { name: 'Create project' }));
    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    expect(await screen.findByText('toko-online')).toBeInTheDocument();
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({ method: 'POST', body: JSON.stringify({ name: 'toko-online' }) });
  });
});
