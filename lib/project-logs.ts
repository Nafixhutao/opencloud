export type LogSource = 'build' | 'runtime' | 'request' | 'platform';
export type LogLevel = 'debug' | 'info' | 'warn' | 'error';

export type ProjectLogEntry = {
  timestamp: string;
  source: LogSource;
  level?: LogLevel;
  message: string;
  service_id?: string;
  deployment_id?: string;
  environment?: string;
  request?: {
    request_id?: string;
    method?: string;
    path?: string;
    status?: number;
    duration_ms?: number;
    response_size?: number;
  };
};

export type ProjectLogFilters = {
  serviceId?: string;
  deploymentId?: string;
  source?: LogSource;
  level?: LogLevel;
  environment?: string;
  search?: string;
  limit?: number;
};

export type ProjectLogsEnvelope = {
  data: ProjectLogEntry[];
  meta: { count: number };
};

export function projectLogsURL(projectId: string, filters: ProjectLogFilters, stream = false) {
  const query = new URLSearchParams();
  if (filters.serviceId) query.set('service_id', filters.serviceId);
  if (filters.deploymentId) query.set('deployment_id', filters.deploymentId);
  if (filters.source) query.set('source', filters.source);
  if (filters.level) query.set('level', filters.level);
  if (filters.environment) query.set('environment', filters.environment);
  if (filters.search) query.set('search', filters.search);
  if (filters.limit) query.set('limit', String(filters.limit));
  const suffix = stream ? '/stream' : '';
  const encoded = query.toString();
  return `/api/projects/${encodeURIComponent(projectId)}/logs${suffix}${encoded ? `?${encoded}` : ''}`;
}

export async function listProjectLogs(projectId: string, filters: ProjectLogFilters): Promise<ProjectLogsEnvelope> {
  const response = await fetch(projectLogsURL(projectId, filters), { cache: 'no-store' });
  const body = (await response.json().catch(() => null)) as ProjectLogsEnvelope | { error?: { message?: string } } | null;
  if (!response.ok) {
    const error = body as { error?: { message?: string } } | null;
    throw new Error(error?.error?.message ?? 'Could not load project logs.');
  }
  return body as ProjectLogsEnvelope;
}
