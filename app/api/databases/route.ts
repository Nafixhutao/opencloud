import { NextResponse } from 'next/server';

import { createDatabaseSchema } from '@/lib/database-validation';
import { proxyAPI } from '@/lib/api-route';

export function GET() {
  return proxyAPI('/api/v1/databases', { method: 'GET' }, 'Could not load databases.');
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
