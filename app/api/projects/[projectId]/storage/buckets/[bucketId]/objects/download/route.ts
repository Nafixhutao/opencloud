import { NextResponse } from 'next/server';

import { apiFetch, ApiError } from '@/lib/api';
import { apiErrorToResponse } from '@/lib/api-route';

type RouteContext = { params: Promise<{ projectId: string; bucketId: string }> };

export async function GET(request: Request, context: RouteContext) {
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
      `/api/v1/projects/${projectId}/storage/buckets/${bucketId}/objects/download?key=${encodeURIComponent(key)}`,
      { method: 'GET' },
    );
    if (!res.ok) {
      const body = await res.json().catch(() => null);
      return NextResponse.json(body, { status: res.status });
    }
    const headers = new Headers();
    res.headers.forEach((value, name) => {
      if (['content-type', 'content-length', 'content-disposition', 'etag'].includes(name)) {
        headers.set(name, value);
      }
    });
    return new NextResponse(res.body, { status: res.status, headers });
  } catch (error) {
    if (error instanceof ApiError) {
      return apiErrorToResponse(error);
    }
    return NextResponse.json(
      { error: { code: 'INTERNAL', message: 'Could not download object.' } },
      { status: 502 },
    );
  }
}
