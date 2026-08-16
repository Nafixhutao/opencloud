export type EnvironmentVariableEnvironment = 'production' | 'preview' | 'development';

export type EnvironmentVariable = {
  id: string;
  key: string;
  value?: string;
  is_secret: boolean;
  environment: EnvironmentVariableEnvironment;
  created_at: string;
  updated_at: string;
};

export type EnvironmentVariableAudit = {
  id: string;
  action: 'created' | 'updated' | 'deleted' | 'revealed' | 'rotated';
  key: string;
  is_secret: boolean;
  environment: string;
  actor_id: string;
  created_at: string;
};

type EnvironmentVariableEnvelope = { data: EnvironmentVariable };
type EnvironmentVariableListEnvelope = { data: EnvironmentVariable[] };
type EnvironmentVariableAuditEnvelope = { data: EnvironmentVariableAudit[] };
type SecretEnvelope = { data: { value: string } };
type ErrorEnvelope = { error?: { code?: string; message?: string } };

export class EnvironmentVariableAPIError extends Error {
  constructor(
    message: string,
    public readonly code: string,
    public readonly status: number,
  ) {
    super(message);
  }
}

async function environmentVariableRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    cache: 'no-store',
    headers: {
      'Content-Type': 'application/json',
      ...init?.headers,
    },
  });
  if (!response.ok) {
    const body = (await response.json().catch(() => null)) as ErrorEnvelope | null;
    throw new EnvironmentVariableAPIError(
      body?.error?.message ?? 'The control plane could not complete this request.',
      body?.error?.code ?? 'INTERNAL',
      response.status,
    );
  }
  // Delete succeeds with 204 No Content — there is no body to parse.
  if (response.status === 204) return undefined as T;
  const body = (await response.json().catch(() => null)) as T | null;
  return (body ?? undefined) as T;
}

export function listEnvironmentVariables(
  projectId: string,
  serviceId: string,
  environment: EnvironmentVariableEnvironment,
): Promise<EnvironmentVariable[]> {
  const query = new URLSearchParams({ environment });
  return environmentVariableRequest<EnvironmentVariableListEnvelope>(
    `/api/projects/${projectId}/services/${serviceId}/environment?${query}`,
  ).then((body) => body?.data ?? []);
}

export function createEnvironmentVariable(
  projectId: string,
  serviceId: string,
  input: { key: string; value: string; is_secret: boolean; environment: EnvironmentVariableEnvironment },
): Promise<EnvironmentVariable> {
  return environmentVariableRequest<EnvironmentVariableEnvelope>(
    `/api/projects/${projectId}/services/${serviceId}/environment`,
    {
      method: 'POST',
      body: JSON.stringify(input),
    },
  ).then((body) => body.data);
}

export function updateEnvironmentVariable(
  projectId: string,
  serviceId: string,
  id: string,
  input: { value: string },
): Promise<EnvironmentVariable> {
  return environmentVariableRequest<EnvironmentVariableEnvelope>(
    `/api/projects/${projectId}/services/${serviceId}/environment/${id}`,
    {
      method: 'PUT',
      body: JSON.stringify(input),
    },
  ).then((body) => body.data);
}

export async function deleteEnvironmentVariable(
  projectId: string,
  serviceId: string,
  id: string,
): Promise<void> {
  await environmentVariableRequest<void>(
    `/api/projects/${projectId}/services/${serviceId}/environment/${id}`,
    { method: 'DELETE' },
  );
}

export function revealSecret(
  projectId: string,
  serviceId: string,
  id: string,
): Promise<string> {
  return environmentVariableRequest<SecretEnvelope>(
    `/api/projects/${projectId}/services/${serviceId}/environment/${id}/reveal`,
    { method: 'POST' },
  ).then((body) => body.data.value);
}

export function listEnvironmentVariableAudit(
  projectId: string,
  serviceId: string,
  limit = 50,
): Promise<EnvironmentVariableAudit[]> {
  const query = new URLSearchParams({ limit: String(limit) });
  return environmentVariableRequest<EnvironmentVariableAuditEnvelope>(
    `/api/projects/${projectId}/services/${serviceId}/environment/audit?${query}`,
  ).then((body) => body?.data ?? []);
}
