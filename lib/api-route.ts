import { NextResponse } from 'next/server';

import { apiFetch } from '@/lib/api';

export async function proxyAPI(
  path: string,
  init: RequestInit,
  fallbackMessage: string,
): Promise<NextResponse> {
  try {
    const response = await apiFetch(path, init);
    const body = await response.json().catch(() => null);
    return NextResponse.json(body, { status: response.status });
  } catch (error) {
    if (error instanceof Error && error.message === 'UNAUTHENTICATED') {
      return NextResponse.json(
        { error: { code: 'UNAUTHENTICATED', message: 'Sign in required' } },
        { status: 401 },
      );
    }
    return NextResponse.json(
      { error: { code: 'INTERNAL', message: fallbackMessage } },
      { status: 502 },
    );
  }
}
