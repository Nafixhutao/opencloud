import { NextResponse } from 'next/server';

import { apiFetch } from '@/lib/api';
import { getSession } from '@/lib/session';
import { memberships } from '@/lib/membership';

type RouteContext = { params: Promise<{ id: string }> };

export async function PATCH(request: Request, context: RouteContext) {
  const session = await getSession();
  if (!session) {
    return NextResponse.json(
      { error: { code: 'UNAUTHENTICATED', message: 'Sign in required' } },
      { status: 401 },
    );
  }

  const membership = await memberships.getByUserId(session.user.id);
  if (!membership || membership.role !== 'admin') {
    return NextResponse.json(
      { error: { code: 'FORBIDDEN', message: 'insufficient permissions' } },
      { status: 403 },
    );
  }

  const { id } = await context.params;
  let body: unknown;
  try {
    body = await request.json();
  } catch {
    return NextResponse.json(
      { error: { code: 'VALIDATION_FAILED', message: 'Invalid JSON body' } },
      { status: 422 },
    );
  }

  try {
    const res = await apiFetch(`/api/v1/admin/users/${id}`, {
      method: 'PATCH',
      body: JSON.stringify(body),
    });
    const json = await res.json().catch(() => null);
    return NextResponse.json(json, { status: res.status });
  } catch (err) {
    const message = err instanceof Error ? err.message : 'API error';
    if (message === 'UNAUTHENTICATED') {
      return NextResponse.json(
        { error: { code: 'UNAUTHENTICATED', message: 'Sign in required' } },
        { status: 401 },
      );
    }
    return NextResponse.json(
      { error: { code: 'INTERNAL', message: 'Could not update user' } },
      { status: 502 },
    );
  }
}
