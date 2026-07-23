import { NextResponse } from 'next/server';

import { apiFetch } from '@/lib/api';
import { profileSchema } from '@/lib/auth-validation';
import { getSession } from '@/lib/session';

export async function PATCH(request: Request) {
  const session = await getSession();
  if (!session) {
    return NextResponse.json(
      { error: { code: 'UNAUTHENTICATED', message: 'Sign in required' } },
      { status: 401 },
    );
  }

  let json: unknown;
  try {
    json = await request.json();
  } catch {
    return NextResponse.json(
      { error: { code: 'VALIDATION_FAILED', message: 'Invalid JSON body' } },
      { status: 422 },
    );
  }

  const parsed = profileSchema.safeParse(json);
  if (!parsed.success) {
    return NextResponse.json(
      {
        error: {
          code: 'VALIDATION_FAILED',
          message: 'Invalid profile',
          details: parsed.error.issues.map((i) => ({
            field: i.path.join('.') || 'name',
            issue: i.message,
          })),
        },
      },
      { status: 422 },
    );
  }

  try {
    const res = await apiFetch('/api/v1/me', {
      method: 'PATCH',
      body: JSON.stringify({ name: parsed.data.name }),
    });
    const body = await res.json().catch(() => null);
    return NextResponse.json(body, { status: res.status });
  } catch (err) {
    const message = err instanceof Error ? err.message : 'API error';
    if (message === 'UNAUTHENTICATED') {
      return NextResponse.json(
        { error: { code: 'UNAUTHENTICATED', message: 'Sign in required' } },
        { status: 401 },
      );
    }
    return NextResponse.json(
      { error: { code: 'INTERNAL', message: 'Could not update profile' } },
      { status: 502 },
    );
  }
}
