export type ConsoleSessionStatus = 'active' | 'expired' | 'revoked';

export type DatabaseEngine = 'postgres' | 'mariadb';

export type DatabaseConsoleSession = {
  id: string;
  expires_at: string;
  token: string;
};

export type CreateConsoleSessionRequest = {
  duration?: number; // in minutes, default 30
};

export type ConsoleSessionsEnvelope = {
  data: DatabaseConsoleSession;
};

export class ConsoleSessionAPIError extends Error {
  constructor(
    message: string,
    public readonly code: string,
    public readonly status: number,
  ) {
    super(message);
  }
}

async function databaseConsoleRequest<T>(
  path: string,
  init?: RequestInit,
): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...init?.headers,
    },
  });

  if (!response.ok) {
    const body = await response.json().catch(() => null);
    throw new ConsoleSessionAPIError(
      body?.error?.message ?? 'The control plane could not complete this request.',
      body?.error?.code ?? 'INTERNAL',
      response.status,
    );
  }

  return (await response.json()) as T;
}

export function createConsoleSession(
  databaseID: string,
  durationMinutes: number = 30,
): Promise<ConsoleSessionsEnvelope> {
  return databaseConsoleRequest<ConsoleSessionsEnvelope>(
    `/api/v1/databases/${databaseID}/console/session`,
    {
      method: 'POST',
      body: JSON.stringify({ duration: `${durationMinutes}m` }),
    },
  );
}

export function revokeConsoleSession(
  databaseID: string,
  sessionID: string,
): Promise<void> {
  return databaseConsoleRequest<void>(
    `/api/v1/databases/${databaseID}/console/session/${sessionID}/revoke`,
    {
      method: 'POST',
    },
  );
}

export type QueryResult = {
  columns: string[];
  rows: Array<Array<string | number>>;
  affected_rows: number;
  elapsed_ms: number;
};

export class ConsoleQueryAPIError extends Error {
  constructor(
    message: string,
    public readonly code: string,
    public readonly status: number,
  ) {
    super(message);
  }
}

async function consoleQueryRequest<T>(
  path: string,
  init?: RequestInit,
): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...init?.headers,
    },
  });

  if (!response.ok) {
    const body = await response.json().catch(() => null);
    throw new ConsoleQueryAPIError(
      body?.error?.message ?? 'The control plane could not complete this request.',
      body?.error?.code ?? 'INTERNAL',
      response.status,
    );
  }

  return (await response.json()) as T;
}

export function executeSQLQuery(
  databaseID: string,
  sessionToken: string,
  query: string,
): Promise<{ data: QueryResult }> {
  return consoleQueryRequest<{ data: QueryResult }>(
    `/api/v1/databases/${databaseID}/console/query`,
    {
      method: 'POST',
      body: JSON.stringify({
        session_token: sessionToken,
        query,
      }),
    },
  );
}
