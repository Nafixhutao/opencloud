import { NextResponse } from 'next/server';

import { createDatabaseSchema } from '@/lib/database-validation';
import { proxyAPI } from '@/lib/api-route';

const defaultPage = 1;
const defaultPerPage = 25;
const maxPerPage = 100;

export function GET(request: Request) {
  const searchParams = new URL(request.url).searchParams;
  const page = positiveInteger(searchParams.get('page'), defaultPage);
  const perPage = Math.min(
    positiveInteger(searchParams.get('per_page'), defaultPerPage),
    maxPerPage,
  );
  const query = new URLSearchParams({
    page: String(page),
    per_page: String(perPage),
  });
  return proxyAPI(
    `/api/v1/databases?${query}`,
    { method: 'GET' },
    'Could not load databases.',
  );
}

export async function POST(request: Request) {
  let json: unknown;
  try {
    json = await request.json();
  } catch {
    return NextResponse.json(
      { error: { code: 'VALIDATION_FAILED', message: 'Invalid JSON body' } },
      { status: 422 },
    );
  }
  const parsed = createDatabaseSchema.safeParse(json);
  if (!parsed.success) {
    return NextResponse.json(
      {
        error: {
          code: 'VALIDATION_FAILED',
          message: 'Check the database details and try again.',
          details: parsed.error.issues.map((issue) => ({
            field: issue.path.join('.') || 'name',
            issue: issue.message,
          })),
        },
      },
      { status: 422 },
    );
  }
  const idempotencyKey = request.headers.get('Idempotency-Key') ?? crypto.randomUUID();
  return proxyAPI(
    '/api/v1/databases',
    {
      method: 'POST',
      headers: { 'Idempotency-Key': idempotencyKey },
      body: JSON.stringify(parsed.data),
    },
    'Could not queue database creation.',
  );
}

function positiveInteger(value: string | null, fallback: number): number {
  if (!value || !/^[1-9]\d*$/.test(value)) {
    return fallback;
  }
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) ? parsed : fallback;
}
