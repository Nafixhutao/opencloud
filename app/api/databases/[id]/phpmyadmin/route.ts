import { NextResponse } from 'next/server';

import { getSession } from '@/lib/session';

type RouteContext = { params: Promise<{ id: string }> };

export async function GET(_request: Request, _context: RouteContext) {
  const session = await getSession();
  if (!session) {
    return NextResponse.json(
      { error: { code: 'UNAUTHENTICATED', message: 'Sign in required' } },
      { status: 401 },
    );
  }

  const phpMyAdminURL = process.env.PHPMYADMIN_URL ?? 'http://localhost:8081';
  return NextResponse.redirect(phpMyAdminURL);
}
