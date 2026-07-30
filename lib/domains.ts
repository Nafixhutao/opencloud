export type DomainStatus =
  | 'pending'
  | 'verifying'
  | 'verified'
  | 'active'
  | 'failed'
  | 'deleting'
  | 'deleted';

export type CertStatus =
  | 'none'
  | 'issuing'
  | 'active'
  | 'expiring'
  | 'revoked'
  | 'error';

export type VerificationType = 'txt';

export type DNSProvider = 'manual' | 'cloudflare';

export type DNSRecord = {
  type: string;
  name: string;
  content: string;
  ttl: number;
};

export type VerificationInstructions = {
  type: string;
  records: DNSRecord[];
};

export type Domain = {
  id: string;
  site_id?: string;
  hostname: string;
  status: DomainStatus;
  verification_type?: VerificationType;
  verified_at?: string;
  dns_provider: DNSProvider;
  cert_status: CertStatus;
  cert_expires_at?: string;
  cert_auto_renew: boolean;
  last_error?: string;
  created_at: string;
  updated_at: string;
};

export type DomainEnvelope = { data: Domain };
export type DomainsListEnvelope = { data: Domain[] };
export type InstructionsEnvelope = { data: VerificationInstructions };

type ErrorEnvelope = { error?: { code?: string; message?: string } };

export class DomainAPIError extends Error {
  constructor(
    message: string,
    public readonly code: string,
    public readonly status: number,
  ) {
    super(message);
  }
}

async function domainRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  });
  const body = (await response.json().catch(() => null)) as T | ErrorEnvelope | null;
  if (!response.ok) {
    const error = body as ErrorEnvelope | null;
    throw new DomainAPIError(
      error?.error?.message ?? 'The control plane could not complete this request.',
      error?.error?.code ?? 'INTERNAL',
      response.status,
    );
  }
  return body as T;
}

export function listDomains(siteID: string): Promise<DomainsListEnvelope> {
  return domainRequest<DomainsListEnvelope>(`/api/sites/${siteID}/domains`);
}

export function attachDomain(
  siteID: string,
  hostname: string,
): Promise<DomainEnvelope> {
  return domainRequest<DomainEnvelope>(`/api/sites/${siteID}/domains`, {
    method: 'POST',
    body: JSON.stringify({ hostname }),
  });
}

export function getDomainInstructions(domainID: string): Promise<InstructionsEnvelope> {
  return domainRequest<InstructionsEnvelope>(`/api/domains/${domainID}/instructions`);
}

export function verifyDomain(domainID: string): Promise<DomainEnvelope> {
  return domainRequest<DomainEnvelope>(`/api/domains/${domainID}/verify`, {
    method: 'POST',
  });
}

export function detachDomain(domainID: string): Promise<DomainEnvelope> {
  return domainRequest<DomainEnvelope>(`/api/domains/${domainID}`, {
    method: 'DELETE',
  });
}

export function isDomainTransitional(status: DomainStatus): boolean {
  return ['pending', 'verifying', 'deleting'].includes(status);
}