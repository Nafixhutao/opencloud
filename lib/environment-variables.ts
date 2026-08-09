import { apiClient } from './api-client';

export interface EnvironmentVariable {
  id: string;
  key: string;
  value?: string;
  is_secret: boolean;
  environment: 'production' | 'preview' | 'development';
  created_at: string;
  updated_at: string;
}

export interface CreateEnvironmentVariableInput {
  key: string;
  value: string;
  is_secret: boolean;
  environment: string;
}

export interface UpdateEnvironmentVariableInput {
  value: string;
}

export interface EnvironmentVariableAudit {
  id: string;
  action: 'created' | 'updated' | 'deleted' | 'revealed' | 'rotated';
  key: string;
  is_secret: boolean;
  environment: string;
  actor_id: string;
  created_at: string;
}

export async function listEnvironmentVariables(
  projectId: string,
  serviceId: string,
  environment: string
): Promise<EnvironmentVariable[]> {
  const response = await apiClient(
    `/projects/${projectId}/services/${serviceId}/environment?environment=${environment}`
  );
  if (!response.ok) {
    throw new Error('Failed to fetch environment variables');
  }
  const data = await response.json();
  return data.data || [];
}

export async function createEnvironmentVariable(
  projectId: string,
  serviceId: string,
  input: CreateEnvironmentVariableInput
): Promise<EnvironmentVariable> {
  const response = await apiClient(
    `/projects/${projectId}/services/${serviceId}/environment`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    }
  );
  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error?.message || 'Failed to create environment variable');
  }
  const data = await response.json();
  return data.data;
}

export async function updateEnvironmentVariable(
  projectId: string,
  serviceId: string,
  id: string,
  input: UpdateEnvironmentVariableInput
): Promise<EnvironmentVariable> {
  const response = await apiClient(
    `/projects/${projectId}/services/${serviceId}/environment/${id}`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    }
  );
  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error?.message || 'Failed to update environment variable');
  }
  const data = await response.json();
  return data.data;
}

export async function deleteEnvironmentVariable(
  projectId: string,
  serviceId: string,
  id: string
): Promise<void> {
  const response = await apiClient(
    `/projects/${projectId}/services/${serviceId}/environment/${id}`,
    {
      method: 'DELETE',
    }
  );
  if (!response.ok) {
    throw new Error('Failed to delete environment variable');
  }
}

export async function revealSecret(
  projectId: string,
  serviceId: string,
  id: string
): Promise<string> {
  const response = await apiClient(
    `/projects/${projectId}/services/${serviceId}/environment/${id}/reveal`,
    {
      method: 'POST',
    }
  );
  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error?.message || 'Failed to reveal secret');
  }
  const data = await response.json();
  return data.data.value;
}

export async function listEnvironmentVariableAudit(
  projectId: string,
  serviceId: string,
  limit = 50
): Promise<EnvironmentVariableAudit[]> {
  const response = await apiClient(
    `/projects/${projectId}/services/${serviceId}/environment/audit?limit=${limit}`
  );
  if (!response.ok) {
    throw new Error('Failed to fetch audit trail');
  }
  const data = await response.json();
  return data.data || [];
}
