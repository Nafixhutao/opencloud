// Database console session management types and APIs

export interface DatabaseConsoleSession {
  id: string;
  account_id: string;
  database_id: string;
  created_at: string;
  expires_at: string;
  ip_addr?: string;
  user_agent?: string;
  last_activity_at?: string;
}

export interface SessionCreateRequest {
  ttlMinutes?: number; // 15, 30, or 60 minutes
}

export interface QueryExecuteRequest {
  sessionId: string;
  query: string;
  maxRows?: number;
  timeoutSeconds?: number;
  disallowMultiStatement?: boolean;
}

export interface QueryResult {
  status: string;
  columns?: string[];
  rows?: any[][];
  rows_affected?: number;
  execution_time_sec?: number;
}

export async function createConsoleSession(
  databaseId: string,
  options?: SessionCreateRequest
): Promise<DatabaseConsoleSession> {
  const response = await fetch(`/api/v1/databases/${databaseId}/console/sessions`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(options || {}),
  });

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.message || "Failed to create session");
  }

  const data = await response.json();
  return data.data;
}

export async function revokeConsoleSession(
  databaseId: string,
  sessionId: string
): Promise<void> {
  const response = await fetch(
    `/api/v1/databases/${databaseId}/console/sessions/${sessionId}`,
    {
      method: "DELETE",
    }
  );

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.message || "Failed to revoke session");
  }
}

export async function executeConsoleQuery(
  databaseId: string,
  request: QueryExecuteRequest
): Promise<QueryResult> {
  const response = await fetch(
    `/api/v1/databases/${databaseId}/console/execute`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(request),
    }
  );

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.message || "Query execution failed");
  }

  const data = await response.json();
  return data.data;
}
