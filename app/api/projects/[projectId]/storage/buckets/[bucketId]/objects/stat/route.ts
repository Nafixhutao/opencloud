import { NextResponse } from 'next/server';

import { apiFetch, ApiError } from '@/lib/api';
import { apiErrorToResponse } from '@/lib/api-route';

type RouteContext = { params: Promise<{ projectId: string; bucketId: string }> };

export async function HEAD(request: Request, context: RouteContext) {
  const { projectId, bucketId } = await context.params;
  const key = new URL(request.url).searchParams.get('key');
  if (!key) {
    return NextResponse.json(
      { error: { code: 'VALIDATION_FAILED', message: 'Object key is required' } },
      { status: 422 },
    );
  }
  try {
    const res = await apiFetch(
      `/api/v1/projects/${projectId}/storage/buckets/${bucketId}/objects/stat?key=${encodeURIComponent(key)}`,
      { method: 'HEAD' },
    );
    const body = await res.json().catch(() => null);
    return NextResponse.json(body, { status: res.status });
  } catch (error) {
    if (error instanceof ApiError) {
      return apiErrorToResponse(error);
    }
    return NextResponse.json(
      { error: { code: 'INTERNAL', message: 'Could not fetch object metadata.' } },
      { status: 502 },
    );
  }
}
