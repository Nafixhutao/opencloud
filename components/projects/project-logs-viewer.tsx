'use client';

import { Pause, Play, RefreshCw, Search } from 'lucide-react';
import { useCallback, useEffect, useRef, useState } from 'react';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { cn } from '@/lib/utils';
import { listProjectLogs, projectLogsURL, type LogLevel, type LogSource, type ProjectLogEntry, type ProjectLogFilters } from '@/lib/project-logs';
import type { ProjectService } from '@/lib/projects';

const maximumVisibleLogs = 1000;

type ProjectLogsViewerProps = {
  projectId: string;
  services: ProjectService[];
};

export function ProjectLogsViewer({ projectId, services }: ProjectLogsViewerProps) {
  const [draft, setDraft] = useState<ProjectLogFilters>({ limit: 200 });
  const [filters, setFilters] = useState<ProjectLogFilters>({ limit: 200 });
  const [entries, setEntries] = useState<ProjectLogEntry[]>([]);
  const [live, setLive] = useState(true);
  const [connection, setConnection] = useState<'connecting' | 'live' | 'paused' | 'retrying'>('connecting');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [autoscroll, setAutoscroll] = useState(true);
  const [wrap, setWrap] = useState(true);
  const [timestamps, setTimestamps] = useState(true);
  const endRef = useRef<HTMLDivElement>(null);

  const load = useCallback(async (nextFilters: ProjectLogFilters) => {
    setLoading(true);
    setError(null);
    try {
      const result = await listProjectLogs(projectId, nextFilters);
      setEntries(result.data.slice(-maximumVisibleLogs));
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : 'Could not load project logs.');
    } finally {
      setLoading(false);
    }
  }, [projectId]);

  useEffect(() => { void load(filters); }, [filters, load]);

  useEffect(() => {
    if (!live) {
      setConnection('paused');
      return;
    }
    setConnection('connecting');
    const source = new EventSource(projectLogsURL(projectId, filters, true));
    source.onopen = () => setConnection('live');
    source.onerror = () => setConnection('retrying');
    source.addEventListener('log', (event) => {
      try {
        const entry = JSON.parse((event as MessageEvent<string>).data) as ProjectLogEntry;
        setEntries((current) => appendUniqueLog(current, entry));
      } catch {
        setConnection('retrying');
      }
    });
    return () => source.close();
  }, [filters, live, projectId]);

  useEffect(() => {
    if (autoscroll && typeof endRef.current?.scrollIntoView === 'function') {
      endRef.current.scrollIntoView({ block: 'nearest' });
    }
  }, [autoscroll, entries]);

  function applyFilters() {
    setFilters({
      ...draft,
      environment: draft.environment?.trim() || undefined,
      search: draft.search?.trim() || undefined,
    });
  }

  return (
    <Card>
      <CardHeader className="gap-3">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <CardTitle>Logs</CardTitle>
            <CardDescription className="mt-1">Build, runtime, request, and platform activity with tenant-safe live tail.</CardDescription>
          </div>
          <div className="flex items-center gap-2">
            <Badge variant={connection === 'live' ? 'default' : 'outline'}>{connectionLabel(connection)}</Badge>
            <Button size="sm" variant="outline" onClick={() => setLive((value) => !value)}>
              {live ? <Pause data-icon="inline-start" /> : <Play data-icon="inline-start" />}
              {live ? 'Pause' : 'Go live'}
            </Button>
            <Button size="icon-sm" variant="ghost" aria-label="Refresh logs" onClick={() => void load(filters)} disabled={loading}>
              <RefreshCw className={cn(loading && 'animate-spin')} />
            </Button>
          </div>
        </div>
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-[minmax(0,1fr)_repeat(4,minmax(0,0.55fr))_auto]">
          <label className="relative">
            <span className="sr-only">Search logs</span>
            <Search aria-hidden="true" className="pointer-events-none absolute left-3 top-3 size-4 text-muted-foreground" />
            <Input className="pl-9" value={draft.search ?? ''} onChange={(event) => setDraft((current) => ({ ...current, search: event.target.value }))} placeholder="Search logs" />
          </label>
          <LogSelect label="Service" value={draft.serviceId ?? ''} onChange={(value) => setDraft((current) => ({ ...current, serviceId: value || undefined, deploymentId: undefined }))}>
            <option value="">All services</option>
            {services.map((service) => <option key={service.id} value={service.id}>{service.name}</option>)}
          </LogSelect>
          <LogSelect label="Source" value={draft.source ?? ''} onChange={(value) => setDraft((current) => ({ ...current, source: (value || undefined) as LogSource | undefined }))}>
            <option value="">All sources</option>
            <option value="build">Build</option><option value="runtime">Runtime</option><option value="request">Requests</option><option value="platform">Platform</option>
          </LogSelect>
          <LogSelect label="Level" value={draft.level ?? ''} onChange={(value) => setDraft((current) => ({ ...current, level: (value || undefined) as LogLevel | undefined }))}>
            <option value="">All levels</option>
            <option value="debug">Debug</option><option value="info">Info</option><option value="warn">Warn</option><option value="error">Error</option>
          </LogSelect>
          <label>
            <span className="sr-only">Environment</span>
            <Input value={draft.environment ?? ''} onChange={(event) => setDraft((current) => ({ ...current, environment: event.target.value }))} placeholder="Environment" />
          </label>
          <Button variant="secondary" onClick={applyFilters}>Apply</Button>
        </div>
        <div className="flex flex-wrap gap-x-5 gap-y-2 text-xs text-muted-foreground">
          <LogToggle label="Autoscroll" checked={autoscroll} onChange={setAutoscroll} />
          <LogToggle label="Wrap lines" checked={wrap} onChange={setWrap} />
          <LogToggle label="Timestamps" checked={timestamps} onChange={setTimestamps} />
          <span className="ml-auto tabular-nums">{entries.length} lines</span>
        </div>
      </CardHeader>
      <CardContent>
        <div role="log" aria-live={live ? 'polite' : 'off'} aria-label="Project logs" className="max-h-[520px] min-h-72 overflow-auto rounded-md border bg-zinc-950 p-4 font-mono text-xs leading-5 text-zinc-200">
          {error ? <p className="text-red-300">{error}</p> : null}
          {!error && !loading && entries.length === 0 ? <p className="text-zinc-500">No logs match these filters yet.</p> : null}
          {entries.map((entry) => <LogLine key={logKey(entry)} entry={entry} services={services} timestamps={timestamps} wrap={wrap} />)}
          <div ref={endRef} />
        </div>
      </CardContent>
    </Card>
  );
}

function LogLine({ entry, services, timestamps, wrap }: { entry: ProjectLogEntry; services: ProjectService[]; timestamps: boolean; wrap: boolean }) {
  const service = services.find((candidate) => candidate.id === entry.service_id)?.name;
  return <div className={cn('flex min-w-max gap-3 border-b border-white/5 py-1 last:border-0', wrap && 'min-w-0 whitespace-pre-wrap break-words')}><span className="flex shrink-0 gap-2 text-zinc-500">{timestamps ? <time dateTime={entry.timestamp}>{new Date(entry.timestamp).toISOString()}</time> : null}<span className="w-16 uppercase text-purple-300">{entry.source}</span>{entry.level ? <span className={levelColor(entry.level)}>{entry.level}</span> : null}{service ? <span>{service}</span> : null}</span><span className={cn(!wrap && 'whitespace-pre')}>{entry.message}</span>{entry.request ? <span className="text-zinc-500">{[entry.request.method, entry.request.path, entry.request.status].filter((value) => value !== undefined && value !== '').join(' ')}</span> : null}</div>;
}

function LogSelect({ label, value, onChange, children }: { label: string; value: string; onChange: (value: string) => void; children: React.ReactNode }) {
  return <label><span className="sr-only">{label}</span><select aria-label={label} value={value} onChange={(event) => onChange(event.target.value)} className="h-10 w-full rounded-sm border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring">{children}</select></label>;
}

function LogToggle({ label, checked, onChange }: { label: string; checked: boolean; onChange: (value: boolean) => void }) {
  return <label className="inline-flex items-center gap-2"><input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} />{label}</label>;
}

function appendUniqueLog(current: ProjectLogEntry[], entry: ProjectLogEntry) {
  const key = logKey(entry);
  if (current.some((candidate) => logKey(candidate) === key)) return current;
  return [...current, entry].slice(-maximumVisibleLogs);
}

function logKey(entry: ProjectLogEntry) {
  return `${entry.timestamp}\u0000${entry.source}\u0000${entry.service_id ?? ''}\u0000${entry.deployment_id ?? ''}\u0000${entry.message}`;
}

function connectionLabel(connection: 'connecting' | 'live' | 'paused' | 'retrying') {
  if (connection === 'live') return 'Live';
  if (connection === 'paused') return 'Paused';
  if (connection === 'retrying') return 'Reconnecting';
  return 'Connecting';
}

function levelColor(level: LogLevel) {
  if (level === 'error') return 'text-red-300';
  if (level === 'warn') return 'text-amber-300';
  if (level === 'debug') return 'text-sky-300';
  return 'text-emerald-300';
}
