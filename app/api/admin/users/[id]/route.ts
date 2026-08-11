import { NextResponse } from 'next/server';

import { apiFetch, ApiError } from '@/lib/api';
import { apiErrorToResponse, withValidatedBody } from '@/lib/api-route';
import { memberships } from '@/lib/auth';
import { getSession } from '@/lib/session';
import { z } from 'zod';

type RouteContext = { params: Promise<{ id: string }> };

// Admin user updates pass through an opaque JSON body — validate it is a
// well-formed object with at least one known field rather than forwarding
// arbitrary JSON to the Go API.
const adminUserPatchSchema = z
  .object({
    role: z.string().optional(),
    status: z.string().optional(),
  })
  .refine((v) => v.role !== undefined || v.status !== undefined, {
    message: 'at least one of role or status is required',
  });

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
  return withValidatedBody(
    request,
    adminUserPatchSchema,
    async (data) => {
      try {
        const res = await apiFetch(`/api/v1/admin/users/${id}`, {
          method: 'PATCH',
          body: JSON.stringify(data),
        });
        const json = await res.json().catch(() => null);
        return NextResponse.json(json, { status: res.status });
      } catch (error) {
        if (error instanceof ApiError) {
          return apiErrorToResponse(error);
        }
        return NextResponse.json(
          { error: { code: 'INTERNAL', message: 'Could not update user' } },
          { status: 502 },
        );
      }
    },
    { message: 'Invalid JSON body' },
  );
}
