export type SiteStatus =
  | 'provisioning'
  | 'active'
  | 'suspending'
  | 'suspended'
  | 'resuming'
  | 'deleting'
  | 'deleted'
  | 'failed';

export type Site = {
  id: string;
  domain: string;
  status: SiteStatus;
  last_error?: string;
  created_at: string;
  updated_at: string;
  deleted_at?: string;
};

export type SitesEnvelope = {
  data: Site[];
  meta: { page: number; per_page: number; total: number };
};

type SiteEnvelope = { data: Site };
type ErrorEnvelope = { error?: { code?: string; message?: string } };

export class SiteAPIError extends Error {
  constructor(
    message: string,
    public readonly code: string,
    public readonly status: number,
  ) {
    super(message);
  }
}

async function siteRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...init?.headers,
    },
  });
  const body = (await response.json().catch(() => null)) as T | ErrorEnvelope | null;
  if (!response.ok) {
    const error = body as ErrorEnvelope | null;
    throw new SiteAPIError(
      error?.error?.message ?? 'The control plane could not complete this request.',
      error?.error?.code ?? 'INTERNAL',
      response.status,
    );
  }
  return body as T;
}

export function listSites(): Promise<SitesEnvelope> {
  return siteRequest<SitesEnvelope>('/api/sites');
}

export function createSite(domain: string, idempotencyKey: string): Promise<SiteEnvelope> {
  return siteRequest<SiteEnvelope>('/api/sites', {
    method: 'POST',
    headers: { 'Idempotency-Key': idempotencyKey },
    body: JSON.stringify({ domain, template: 'static' }),
  });
}

export function suspendSite(siteID: string): Promise<SiteEnvelope> {
  return siteRequest<SiteEnvelope>(`/api/sites/${siteID}/suspend`, { method: 'POST' });
}

export function resumeSite(siteID: string): Promise<SiteEnvelope> {
  return siteRequest<SiteEnvelope>(`/api/sites/${siteID}/resume`, { method: 'POST' });
}

export function deleteSite(siteID: string): Promise<SiteEnvelope> {
  return siteRequest<SiteEnvelope>(`/api/sites/${siteID}`, { method: 'DELETE' });
}

export function hasPendingSites(sites: Site[]): boolean {
  return sites.some((site) =>
    ['provisioning', 'suspending', 'resuming', 'deleting'].includes(site.status),
  );
}
