import { NextResponse } from 'next/server';

import { proxyAPI } from '@/lib/api-route';
import { attachDomainSchema } from '@/lib/domain-validation';

type RouteContext = { params: Promise<{ id: string }> };

export async function GET(_request: Request, context: RouteContext) {
  const { id } = await context.params;
  return proxyAPI(`/api/v1/sites/${id}/domains`, { method: 'GET' }, 'Could not load domains.');
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
  return proxyAPI(
    `/api/v1/sites/${id}/domains`,
    {
      method: 'POST',
      body: JSON.stringify(parsed.data),
    },
    'Could not queue domain attachment.',
  );
}