import { NextResponse } from 'next/server';

import { proxyAPI } from '@/lib/api-route';
import { createProjectSchema } from '@/lib/project-validation';

export function GET() {
  return proxyAPI('/api/v1/projects', { method: 'GET' }, 'Could not load projects.');
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
  const parsed = createProjectSchema.safeParse(json);
  if (!parsed.success) {
    return NextResponse.json(
      {
        error: {
          code: 'VALIDATION_FAILED',
          message: 'Check the project details and try again.',
          details: parsed.error.issues.map((issue) => ({
            field: issue.path.join('.') || 'name',
            issue: issue.message,
          })),
        },
      },
      { status: 422 },
    );
  }
  return proxyAPI(
    '/api/v1/projects',
    {
      method: 'POST',
      headers: { 'Idempotency-Key': request.headers.get('Idempotency-Key') ?? crypto.randomUUID() },
      body: JSON.stringify(parsed.data),
    },
    'Could not create project.',
  );
}
