import { NextResponse } from 'next/server';

import { apiFetch, ApiError } from '@/lib/api';
import { apiErrorToResponse, proxyAPI } from '@/lib/api-route';

type RouteContext = { params: Promise<{ projectId: string; bucketId: string }> };

export async function GET(request: Request, context: RouteContext) {
  const { projectId, bucketId } = await context.params;
  const searchParams = new URL(request.url).searchParams;
  const qs = searchParams.toString();
  return proxyAPI(
    `/api/v1/projects/${projectId}/storage/buckets/${bucketId}/objects${qs ? `?${qs}` : ''}`,
    { method: 'GET' },
    'Could not list objects.',
  );
}

export async function PUT(request: Request, context: RouteContext) {
  const { projectId, bucketId } = await context.params;
  const url = new URL(request.url);
  const key = url.searchParams.get('key');
  if (!key) {
    return NextResponse.json(
      { error: { code: 'VALIDATION_FAILED', message: 'Object key is required' } },
      { status: 422 },
    );
  }
  try {
    const res = await apiFetch(
      `/api/v1/projects/${projectId}/storage/buckets/${bucketId}/objects?key=${encodeURIComponent(key)}`,
      {
        method: 'PUT',
        headers: { 'Content-Type': request.headers.get('Content-Type') ?? 'application/octet-stream' },
        body: await request.arrayBuffer(),
      },
    );
    const body = await res.json().catch(() => null);
    return NextResponse.json(body, { status: res.status });
  } catch (error) {
    if (error instanceof ApiError) {
      return apiErrorToResponse(error);
    }
    return NextResponse.json(
      { error: { code: 'UPLOAD_FAILED', message: 'Upload failed' } },
      { status: 502 },
    );
  }
}

export async function DELETE(request: Request, context: RouteContext) {
  const { projectId, bucketId } = await context.params;
  const key = new URL(request.url).searchParams.get('key');
  if (!key) {
    return NextResponse.json(
      { error: { code: 'VALIDATION_FAILED', message: 'Object key is required' } },
      { status: 422 },
    );
  }
  return proxyAPI(
    `/api/v1/projects/${projectId}/storage/buckets/${bucketId}/objects?key=${encodeURIComponent(key)}`,
    { method: 'DELETE' },
    'Could not delete object.',
  );
}
