import { NextResponse } from 'next/server';

import { proxyAPI } from '@/lib/api-route';
import { createBucketSchema } from '@/lib/bucket-validation';

type RouteContext = { params: Promise<{ projectId: string }> };

export async function GET(request: Request, context: RouteContext) {
  const { projectId } = await context.params;
  const searchParams = new URL(request.url).searchParams;
  const page = Math.min(Math.max(parseInt(searchParams.get('page') ?? '1'), 1), 1000);
  const perPage = Math.min(Math.max(parseInt(searchParams.get('per_page') ?? '25'), 1), 100);
  const query = new URLSearchParams({ page: String(page), per_page: String(perPage) });
  return proxyAPI(
    `/api/v1/projects/${projectId}/storage/buckets?${query}`,
    { method: 'GET' },
    'Could not load storage buckets.',
  );
}

export async function POST(request: Request, context: RouteContext) {
  const { projectId } = await context.params;
  let json: unknown;
  try {
    json = await request.json();
  } catch {
    return NextResponse.json(
      { error: { code: 'VALIDATION_FAILED', message: 'Invalid JSON body' } },
      { status: 422 },
    );
  }
  const parsed = createBucketSchema.safeParse(json);
  if (!parsed.success) {
    return NextResponse.json(
      {
        error: {
          code: 'VALIDATION_FAILED',
          message: 'Check the bucket name and try again.',
          details: parsed.error.issues.map((i) => ({ field: i.path.join('.') || 'name', issue: i.message })),
        },
      },
      { status: 422 },
    );
  }
  const idempotencyKey = request.headers.get('Idempotency-Key') ?? crypto.randomUUID();
  return proxyAPI(
    `/api/v1/projects/${projectId}/storage/buckets`,
    {
      method: 'POST',
      headers: { 'Idempotency-Key': idempotencyKey },
      body: JSON.stringify(parsed.data),
    },
    'Could not queue bucket creation.',
  );
}
