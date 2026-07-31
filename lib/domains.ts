export type DomainStatus =
  | 'pending'
  | 'verifying'
  | 'dns_pending'
  | 'provisioning'
  | 'active'
  | 'failed'
  | 'deleting'
  | 'deleted';

export type CertificateStatus = 'none' | 'issuing' | 'active' | 'expiring' | 'error';

export type Domain = {
  id: string;
  site_id: string;
  hostname: string;
  status: DomainStatus;
  verification_expires_at: string;
  verified_at?: string;
  dns_provider: 'manual' | 'cloudflare';
  cert_status: CertificateStatus;
  cert_expires_at?: string;
  cert_observed_at?: string;
  cert_auto_renew: boolean;
  last_reconciled_at?: string;
  last_error?: string;
  created_at: string;
  updated_at: string;
};

export type DomainsEnvelope = {
  data: Domain[];
  meta: { page: number; per_page: number; total: number };
};

export type DomainEnvelope = { data: Domain };

export type DNSRecord = {
  type: string;
  name: string;
  content: string;
  ttl: number;
};

export type DomainInstructionsEnvelope = {
  data: {
    verification_expires_at: string;
    records: DNSRecord[];
  };
};

export type DomainFieldIssue = { field: string; issue: string };

type ErrorEnvelope = {
  error?: {
    code?: string;
    message?: string;
    details?: DomainFieldIssue[];
  };
};

export class DomainAPIError extends Error {
  constructor(
    message: string,
    public readonly code: string,
    public readonly status: number,
    public readonly details: DomainFieldIssue[] = [],
    public readonly retryAfterSeconds?: number,
  ) {
    super(message);
  }
}

async function domainRequest<T>(path: string, init?: RequestInit): Promise<T> {
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
    throw new DomainAPIError(
      error?.error?.message ?? 'The control plane could not complete this request.',
      error?.error?.code ?? 'INTERNAL',
      response.status,
      error?.error?.details ?? [],
      parseRetryAfter(response.headers.get('Retry-After')),
    );
  }
  return body as T;
}

function parseRetryAfter(value: string | null): number | undefined {
  if (!value) {
    return undefined;
  }
  const seconds = Number(value);
  if (Number.isFinite(seconds) && seconds >= 0) {
    return seconds;
  }
  const at = Date.parse(value);
  if (Number.isNaN(at)) {
    return undefined;
  }
  return Math.max(0, Math.ceil((at - Date.now()) / 1_000));
}

export function listDomains(
  siteID: string,
  page = 1,
  perPage = 25,
): Promise<DomainsEnvelope> {
  const query = page === 1 && perPage === 25
    ? ''
    : `?page=${encodeURIComponent(page)}&per_page=${encodeURIComponent(perPage)}`;
  return domainRequest<DomainsEnvelope>(`/api/sites/${siteID}/domains${query}`);
}

export function attachDomain(
  siteID: string,
  hostname: string,
  idempotencyKey: string,
): Promise<DomainEnvelope> {
  return domainRequest<DomainEnvelope>(`/api/sites/${siteID}/domains`, {
    method: 'POST',
    headers: { 'Idempotency-Key': idempotencyKey },
    body: JSON.stringify({ hostname }),
  });
}

export function getDomainInstructions(domainID: string): Promise<DomainInstructionsEnvelope> {
  return domainRequest<DomainInstructionsEnvelope>(`/api/domains/${domainID}/instructions`);
}

export function verifyDomain(domainID: string): Promise<DomainEnvelope> {
  return domainRequest<DomainEnvelope>(`/api/domains/${domainID}/verify`, { method: 'POST' });
}

export function rotateDomainChallenge(domainID: string): Promise<DomainEnvelope> {
  return domainRequest<DomainEnvelope>(`/api/domains/${domainID}/challenge`, {
    method: 'POST',
  });
}

export function detachDomain(domainID: string): Promise<DomainEnvelope> {
  return domainRequest<DomainEnvelope>(`/api/domains/${domainID}`, { method: 'DELETE' });
}

export function hasPendingDomains(domains: Domain[]): boolean {
  return domains.some(
    (domain) =>
      ['verifying', 'dns_pending', 'provisioning', 'deleting'].includes(domain.status) ||
      domain.cert_status === 'issuing',
  );
}
