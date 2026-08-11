import { NextResponse } from 'next/server';

import { apiFetch, ApiError } from '@/lib/api';
import { apiErrorToResponse, withValidatedBody } from '@/lib/api-route';
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

  return withValidatedBody(
    request,
    profileSchema,
    async (data) => {
      try {
        const res = await apiFetch('/api/v1/me', {
          method: 'PATCH',
          body: JSON.stringify({ name: data.name }),
        });
        const body = await res.json().catch(() => null);
        return NextResponse.json(body, { status: res.status });
      } catch (error) {
        if (error instanceof ApiError) {
          return apiErrorToResponse(error);
        }
        return NextResponse.json(
          { error: { code: 'INTERNAL', message: 'Could not update profile' } },
          { status: 502 },
        );
      }
    },
    { message: 'Invalid profile' },
  );
}
