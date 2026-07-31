import { NextResponse } from 'next/server';

import { proxyAPI } from '@/lib/api-route';
import { attachDomainSchema } from '@/lib/domain-validation';

type RouteContext = { params: Promise<{ id: string }> };

function positiveInteger(value: string | null, fallback: number, maximum: number) {
  if (!value || !/^\d+$/u.test(value)) {
    return fallback;
  }
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) && parsed > 0
    ? Math.min(parsed, maximum)
    : fallback;
}

export async function GET(request: Request, context: RouteContext) {
  const { id } = await context.params;
  const incoming = new URL(request.url);
  const page = positiveInteger(incoming.searchParams.get('page'), 1, 1_000_000);
  const perPage = positiveInteger(incoming.searchParams.get('per_page'), 25, 100);
  const query = page === 1 && perPage === 25 ? '' : `?page=${page}&per_page=${perPage}`;
  return proxyAPI(
    `/api/v1/sites/${id}/domains${query}`,
    { method: 'GET' },
    'Could not load domains.',
  );
}

export async function POST(request: Request, context: RouteContext) {
  const { id } = await context.params;
  let json: unknown;
  try {
    json = await request.json();
  } catch {
    return NextResponse.json(
      { error: { code: 'VALIDATION_FAILED', message: 'Invalid JSON body' } },
      { status: 422 },
    );
  }
  const parsed = attachDomainSchema.safeParse(json);
  if (!parsed.success) {
    return NextResponse.json(
      {
        error: {
          code: 'VALIDATION_FAILED',
          message: 'Check the hostname and try again.',
          details: parsed.error.issues.map((issue) => ({
            field: issue.path.join('.') || 'hostname',
            issue: issue.message,
          })),
        },
      },
      { status: 422 },
    );
  }
  const idempotencyKey = request.headers.get('Idempotency-Key') ?? crypto.randomUUID();
  return proxyAPI(
    `/api/v1/sites/${id}/domains`,
    {
      method: 'POST',
      headers: { 'Idempotency-Key': idempotencyKey },
      body: JSON.stringify(parsed.data),
    },
    'Could not attach the domain.',
  );
}
