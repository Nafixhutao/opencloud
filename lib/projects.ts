export type ProjectStatus = 'active' | 'archived' | 'deleted';

export type Project = {
  id: string;
  name: string;
  status: ProjectStatus;
  created_at: string;
  updated_at: string;
};

export type ProjectService = {
  id: string;
  name: string;
  type: 'web' | 'worker' | 'cron' | 'static';
  source_root: string;
  git_repo_url: string;
  git_branch: string;
  status: 'active' | 'disabled' | 'deleted';
  created_at: string;
  updated_at: string;
};

export type ProjectsEnvelope = {
  data: Project[];
  meta: { page: number; per_page: number; total: number };
};

export type ProjectServicesEnvelope = {
  data: ProjectService[];
  meta: { page: number; per_page: number; total: number };
};

type ProjectEnvelope = { data: Project };
type ErrorEnvelope = { error?: { code?: string; message?: string } };

export class ProjectAPIError extends Error {
  constructor(
    message: string,
    public readonly code: string,
    public readonly status: number,
  ) {
    super(message);
  }
}

async function projectRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  });
  const body = (await response.json().catch(() => null)) as T | ErrorEnvelope | null;
  if (!response.ok) {
    const error = body as ErrorEnvelope | null;
    throw new ProjectAPIError(
      error?.error?.message ?? 'The control plane could not complete this request.',
      error?.error?.code ?? 'INTERNAL',
      response.status,
    );
  }
  return body as T;
}

export function listProjects(): Promise<ProjectsEnvelope> {
  return projectRequest<ProjectsEnvelope>('/api/projects');
}

export function createProject(name: string, idempotencyKey: string): Promise<ProjectEnvelope> {
  return projectRequest<ProjectEnvelope>('/api/projects', {
    method: 'POST',
    headers: { 'Idempotency-Key': idempotencyKey },
    body: JSON.stringify({ name }),
  });
}
