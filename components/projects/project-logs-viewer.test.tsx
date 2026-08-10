import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { ProjectLogsViewer } from '@/components/projects/project-logs-viewer';
import type { ProjectService } from '@/lib/projects';

class FakeEventSource {
  static instances: FakeEventSource[] = [];
  readonly url: string;
  closed = false;
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  private listeners = new Map<string, ((event: MessageEvent<string>) => void)[]>();

  constructor(url: string | URL) {
    this.url = String(url);
    FakeEventSource.instances.push(this);
  }

  addEventListener(name: string, listener: EventListenerOrEventListenerObject) {
    const callback = listener as (event: MessageEvent<string>) => void;
    this.listeners.set(name, [...(this.listeners.get(name) ?? []), callback]);
  }

  close() { this.closed = true; }

  emit(name: string, data: unknown) {
    const event = new MessageEvent(name, { data: JSON.stringify(data) });
    for (const listener of this.listeners.get(name) ?? []) listener(event);
  }
}

const services: ProjectService[] = [{
  id: 'e78ea04c-e974-4c9d-b8eb-bcf4df504b93',
  name: 'api',
  type: 'web',
  source_root: '',
  git_repo_url: '',
  git_branch: '',
  storage_persist_bytes: 0,
  status: 'active',
  created_at: '2026-08-09T00:00:00Z',
  updated_at: '2026-08-09T00:00:00Z',
}];

beforeEach(() => {
  FakeEventSource.instances = [];
  vi.stubGlobal('EventSource', FakeEventSource as unknown as typeof EventSource);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('ProjectLogsViewer', () => {
  it('loads history and appends live SSE entries', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({
      data: [{ timestamp: '2026-08-09T12:00:00Z', source: 'platform', level: 'info', message: 'Deployment queued' }],
      meta: { count: 1 },
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }));
    render(<ProjectLogsViewer projectId="8d04424f-620e-41b7-9475-ecb0d6e2a582" services={services} />);
    expect(await screen.findByText('Deployment queued')).toBeInTheDocument();
    expect(FakeEventSource.instances).toHaveLength(1);
    FakeEventSource.instances[0]?.emit('log', { timestamp: '2026-08-09T12:00:01Z', source: 'runtime', level: 'info', message: 'server ready', service_id: services[0]?.id });
    expect(await screen.findByText('server ready')).toBeInTheDocument();
    expect(screen.getAllByText('api')).toHaveLength(2);
  });

  it('pauses the live stream and applies encoded filters', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ data: [], meta: { count: 0 } }), { status: 200, headers: { 'Content-Type': 'application/json' } }));
    render(<ProjectLogsViewer projectId="8d04424f-620e-41b7-9475-ecb0d6e2a582" services={services} />);
    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    fireEvent.click(screen.getByRole('button', { name: 'Pause' }));
    expect(FakeEventSource.instances[0]?.closed).toBe(true);
    expect(await screen.findByText('Paused')).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText('Search logs'), { target: { value: 'order failed' } });
    fireEvent.change(screen.getByLabelText('Service'), { target: { value: services[0]?.id } });
    fireEvent.change(screen.getByLabelText('Source'), { target: { value: 'runtime' } });
    fireEvent.click(screen.getByRole('button', { name: 'Apply' }));
    await waitFor(() => {
      const url = String(fetchMock.mock.calls.at(-1)?.[0]);
      expect(url).toContain(`service_id=${services[0]?.id}`);
      expect(url).toContain('source=runtime');
      expect(url).toContain('search=order+failed');
    });
  });
});
