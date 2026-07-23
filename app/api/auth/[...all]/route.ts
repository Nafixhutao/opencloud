import { createHash } from 'node:crypto';

import { toNextJsHandler } from 'better-auth/next-js';

import { auth, memberships } from '../../../../lib/auth';

const handlers = toNextJsHandler(auth);

function authPath(request: Request): string {
  const pathname = new URL(request.url).pathname;
  return pathname.replace(/^\/api\/auth/, '') || '/';
}

function tokenExpiry(token: string): Date {
  try {
    const payload = token.split('.')[1];
    if (!payload) {
      throw new Error('missing payload');
    }
    const parsed = JSON.parse(Buffer.from(payload, 'base64url').toString('utf8')) as {
      exp?: unknown;
    };
    if (typeof parsed.exp === 'number' && Number.isFinite(parsed.exp)) {
      return new Date(parsed.exp * 1000);
    }
  } catch {
    // Better Auth performs authoritative signature/expiry validation.
  }
  return new Date(Date.now() + 60 * 60 * 1000);
}

function invalidVerificationResponse(request: Request): Response {
  const requestURL = new URL(request.url);
  const callback = requestURL.searchParams.get('callbackURL');
  if (callback) {
    const target = new URL(callback, requestURL.origin);
    if (target.origin === requestURL.origin) {
      target.searchParams.set('error', 'INVALID_TOKEN');
      return Response.redirect(target, 302);
    }
  }
  return Response.json(
    { error: { code: 'INVALID_TOKEN', message: 'Verification link is invalid or used' } },
    { status: 401 },
  );
}

export async function GET(request: Request): Promise<Response> {
  if (authPath(request) !== '/verify-email') {
    return handlers.GET(request);
  }

  const token = new URL(request.url).searchParams.get('token');
  if (!token) {
    return handlers.GET(request);
  }
  const tokenHash = createHash('sha256').update(token).digest();
  const claimed = await memberships.claimEmailVerificationToken(
    tokenHash,
    tokenExpiry(token),
  );
  if (!claimed) {
    return invalidVerificationResponse(request);
  }

  try {
    const response = await handlers.GET(request);
    const location = response.headers.get('location');
    const failedRedirect = location?.includes('error=') ?? false;
    if (!response.ok && !response.redirected && !location) {
      await memberships.releaseEmailVerificationToken(tokenHash);
    } else if (failedRedirect) {
      await memberships.releaseEmailVerificationToken(tokenHash);
    }
    return response;
  } catch (error) {
    await memberships.releaseEmailVerificationToken(tokenHash);
    throw error;
  }
}

export async function POST(request: Request): Promise<Response> {
  const path = authPath(request);
  const session =
    path === '/change-password'
      ? await auth.api.getSession({ headers: request.headers })
      : null;

  const response = await handlers.POST(request);
  if (path === '/sign-in/email' && !response.ok) {
    try {
      await memberships.appendAudit({
        action: 'auth.login.failure',
        metadata: { status: response.status },
      });
    } catch {
      return Response.json(
        { error: { code: 'UNAVAILABLE', message: 'Authentication audit unavailable' } },
        { status: 503 },
      );
    }
  }

  if (path === '/change-password' && response.ok && session?.user) {
    try {
      await memberships.appendAudit({
        accountId:
          (await memberships.getByUserId(session.user.id))?.account_id ?? null,
        actorId: session.user.id,
        action: 'auth.password.change',
        target: session.user.id,
      });
    } catch {
      // Better Auth owns the credential transaction. Returning failure makes the
      // cross-boundary mutation explicitly incomplete instead of claiming an
      // unaudited success; callers must re-authenticate before retrying.
      return Response.json(
        { error: { code: 'AUDIT_FAILED', message: 'Password changed but audit failed' } },
        { status: 500 },
      );
    }
  }
  return response;
}
