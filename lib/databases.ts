export type DatabaseEngine = 'postgres' | 'mariadb';
export type DatabaseStatus = 'provisioning' | 'active' | 'deleting' | 'deleted' | 'failed';

export type ManagedDatabase = {
  id: string;
  name: string;
  engine: DatabaseEngine;
  status: DatabaseStatus;
  credential_available: boolean;
  last_error?: string;
  created_at: string;
  updated_at: string;
  deleted_at?: string;
};

export type DatabasesEnvelope = {
  data: ManagedDatabase[];
  meta: { page: number; per_page: number; total: number };
};

export type DatabaseCredentials = {
  engine: DatabaseEngine;
  host: string;
  port: number;
  database: string;
  username: string;
  password: string;
  tls_required: boolean;
};

type DatabaseEnvelope = { data: ManagedDatabase };
type CredentialEnvelope = { data: DatabaseCredentials };
type ErrorEnvelope = { error?: { code?: string; message?: string } };

export class DatabaseAPIError extends Error {
  constructor(
    message: string,
    public readonly code: string,
    public readonly status: number,
  ) {
    super(message);
  }
}

async function databaseRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    cache: 'no-store',
    headers: {
      'Content-Type': 'application/json',
      ...init?.headers,
    },
  });
  const body = (await response.json().catch(() => null)) as T | ErrorEnvelope | null;
  if (!response.ok) {
    const error = body as ErrorEnvelope | null;
    throw new DatabaseAPIError(
      error?.error?.message ?? 'The control plane could not complete this request.',
      error?.error?.code ?? 'INTERNAL',
      response.status,
    );
  }
  return body as T;
}

export function listDatabases(): Promise<DatabasesEnvelope> {
  return databaseRequest<DatabasesEnvelope>('/api/databases');
}

export function createDatabase(
  name: string,
  engine: DatabaseEngine,
  idempotencyKey: string,
): Promise<DatabaseEnvelope> {
  return databaseRequest<DatabaseEnvelope>('/api/databases', {
    method: 'POST',
    headers: { 'Idempotency-Key': idempotencyKey },
    body: JSON.stringify({ name, engine }),
  });
}

export function deleteDatabase(databaseID: string): Promise<DatabaseEnvelope> {
  return databaseRequest<DatabaseEnvelope>(`/api/databases/${databaseID}`, {
    method: 'DELETE',
  });
}

export function revealDatabaseCredentials(databaseID: string): Promise<CredentialEnvelope> {
  return databaseRequest<CredentialEnvelope>(
    `/api/databases/${databaseID}/credentials/reveal`,
    { method: 'POST' },
  );
}

export function hasPendingDatabases(databases: ManagedDatabase[]): boolean {
  return databases.some((database) =>
    ['provisioning', 'deleting'].includes(database.status),
  );
}
