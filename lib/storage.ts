export type BucketStatus = 'creating' | 'active' | 'deleting' | 'deleted' | 'failed';
export type BucketVisibility = 'public' | 'private';

export type StorageBucket = {
  id: string;
  name: string;
  physical_name: string;
  visibility: BucketVisibility;
  status: BucketStatus;
  storage_limit_bytes: number;
  max_object_size_bytes: number;
  object_count: number;
  allowed_mime_types: string[];
  created_at: string;
};

export type ObjectInfo = {
  key: string;
  size: number;
  content_type?: string;
  etag?: string;
  last_modified: string;
};

export type BucketListEnvelope = {
  data: StorageBucket[];
  meta: { page: number; per_page: number; total: number };
};

export type BucketEnvelope = { data: StorageBucket };
export type ObjectListEnvelope = { data: ObjectInfo[]; next_continuation_token?: string };
export type ObjectUploadEnvelope = {
  data: { id: string; key: string; size: number; content_type: string; etag: string; created_at: string };
};
export type DeleteEnvelope = { deleted: boolean };
export type PresignedUrlEnvelope = { data: { url: string; expires_in_seconds: number } };
export type ErrorEnvelope = { error?: { code?: string; message?: string } };

export class StorageAPIError extends Error {
  constructor(
    message: string,
    public readonly code: string,
    public readonly status: number,
  ) {
    super(message);
  }
}

async function storageRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    cache: 'no-store',
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  });
  const body = (await response.json().catch(() => null)) as T | ErrorEnvelope | null;
  if (!response.ok) {
    const error = body as ErrorEnvelope | null;
    throw new StorageAPIError(
      error?.error?.message ?? 'The control plane could not complete this request.',
      error?.error?.code ?? 'INTERNAL',
      response.status,
    );
  }
  return body as T;
}

export function listBuckets(
  projectId: string,
  params: { page: number; perPage: number },
): Promise<BucketListEnvelope> {
  const query = new URLSearchParams({ page: String(params.page), per_page: String(params.perPage) });
  return storageRequest<BucketListEnvelope>(
    `/api/projects/${projectId}/storage/buckets?${query}`,
    { method: 'GET' },
  );
}

export function createBucket(
  projectId: string,
  name: string,
  idempotencyKey: string,
): Promise<BucketEnvelope> {
  return storageRequest<BucketEnvelope>(
    `/api/projects/${projectId}/storage/buckets`,
    {
      method: 'POST',
      headers: { 'Idempotency-Key': idempotencyKey },
      body: JSON.stringify({ name }),
    },
  );
}

export function deleteBucket(
  projectId: string,
  bucketId: string,
): Promise<BucketEnvelope> {
  return storageRequest<BucketEnvelope>(
    `/api/projects/${projectId}/storage/buckets/${bucketId}`,
    { method: 'DELETE' },
  );
}

export function listObjects(
  projectId: string,
  bucketId: string,
  params: { prefix?: string; continuationToken?: string; limit?: number },
): Promise<ObjectListEnvelope> {
  const query = new URLSearchParams();
  if (params.prefix) query.set('prefix', params.prefix);
  if (params.continuationToken) query.set('continuation_token', params.continuationToken);
  if (params.limit) query.set('limit', String(params.limit));
  const qs = query.toString();
  return storageRequest<ObjectListEnvelope>(
    `/api/projects/${projectId}/storage/buckets/${bucketId}/objects${qs ? `?${qs}` : ''}`,
    { method: 'GET' },
  );
}

export async function uploadObject(
  projectId: string,
  bucketId: string,
  key: string,
  file: File,
): Promise<ObjectUploadEnvelope> {
  const response = await fetch(
    `/api/projects/${projectId}/storage/buckets/${bucketId}/objects?key=${encodeURIComponent(key)}`,
    {
      method: 'PUT',
      cache: 'no-store',
      headers: { 'Content-Type': file.type || 'application/octet-stream' },
      body: file,
    },
  );
  const body = await response.json().catch(() => null);
  if (!response.ok) {
    const error = body as ErrorEnvelope | null;
    throw new StorageAPIError(
      error?.error?.message ?? 'Upload failed.',
      error?.error?.code ?? 'INTERNAL',
      response.status,
    );
  }
  return body as ObjectUploadEnvelope;
}

export function deleteObject(
  projectId: string,
  bucketId: string,
  key: string,
): Promise<DeleteEnvelope> {
  return storageRequest<DeleteEnvelope>(
    `/api/projects/${projectId}/storage/buckets/${bucketId}/objects?key=${encodeURIComponent(key)}`,
    { method: 'DELETE' },
  );
}

export function getDownloadUrl(
  projectId: string,
  bucketId: string,
  key: string,
): string {
  return `/api/projects/${projectId}/storage/buckets/${bucketId}/objects/download?key=${encodeURIComponent(key)}`;
}

export function getPresignedGetUrl(
  projectId: string,
  bucketId: string,
  key: string,
  expirySeconds?: number,
): Promise<PresignedUrlEnvelope> {
  const query = new URLSearchParams({ key });
  if (expirySeconds) query.set('expiry', `${expirySeconds}s`);
  return storageRequest<PresignedUrlEnvelope>(
    `/api/projects/${projectId}/storage/buckets/${bucketId}/presigned-get?${query}`,
    { method: 'GET' },
  );
}

export function hasPendingBuckets(buckets: StorageBucket[]): boolean {
  return buckets.some((b) => ['creating', 'deleting'].includes(b.status));
}
