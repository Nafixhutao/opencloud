import { NextResponse } from 'next/server';

import { proxyAPI } from '@/lib/api-route';
import { createSiteSchema } from '@/lib/site-validation';

export function GET() {
  return proxyAPI('/api/v1/sites', { method: 'GET' }, 'Could not load sites.');
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
  const parsed = createSiteSchema.safeParse(json);
  if (!parsed.success) {
    return NextResponse.json(
      {
        error: {
          code: 'VALIDATION_FAILED',
          message: 'Check the site details and try again.',
          details: parsed.error.issues.map((issue) => ({
            field: issue.path.join('.') || 'domain',
            issue: issue.message,
          })),
        },
      },
      { status: 422 },
    );
  }
  const idempotencyKey = request.headers.get('Idempotency-Key') ?? crypto.randomUUID();
  return proxyAPI(
    '/api/v1/sites',
    {
      method: 'POST',
      headers: { 'Idempotency-Key': idempotencyKey },
      body: JSON.stringify(parsed.data),
    },
    'Could not queue site creation.',
  );
}
