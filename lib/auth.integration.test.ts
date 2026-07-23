import assert from 'node:assert/strict';
import test from 'node:test';

const databaseURL = process.env.DATABASE_URL;

type AuthRuntime = {
  auth: (typeof import('./auth.ts'))['auth'];
  authPool: (typeof import('./auth.ts'))['authPool'];
  domainPool: (typeof import('./auth.ts'))['domainPool'];
  mail: (typeof import('./auth.ts'))['mail'];
  GET: (request: Request) => Promise<Response>;
  POST: (request: Request) => Promise<Response>;
};

async function loadRuntime(): Promise<AuthRuntime> {
  (process.env as Record<string, string | undefined>).NODE_ENV = 'test';
  process.env.ENV = 'test';
  process.env.MAIL_PROVIDER = 'memory';
  process.env.BETTER_AUTH_SECRET ??= 'integration-secret-at-least-32-characters';
  process.env.BETTER_AUTH_URL ??= 'http://localhost:3000';
  const authModule = await import('./auth.ts');
  const route = await import('../app/api/auth/[...all]/route.ts');
  return {
    auth: authModule.auth,
    authPool: authModule.authPool,
    domainPool: authModule.domainPool,
    mail: authModule.mail,
    GET: route.GET,
    POST: route.POST,
  };
}

function jsonRequest(path: string, body: unknown, cookie?: string): Request {
  const headers = new Headers({
    'Content-Type': 'application/json',
    Origin: 'http://localhost:3000',
  });
  if (cookie) {
    headers.set('Cookie', cookie);
  }
  return new Request(`http://localhost:3000/api/auth${path}`, {
    method: 'POST',
    headers,
    body: JSON.stringify(body),
  });
}

async function waitForMessage(
  runtime: AuthRuntime,
  kind: string,
  email: string,
  fromIndex: number,
): Promise<string> {
  for (let attempt = 0; attempt < 100; attempt++) {
    const message = runtime.mail.sent
      ?.slice(fromIndex)
      .find((entry) => entry.to === email && entry.tags?.kind === kind);
    if (message) {
      return message.text;
    }
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  throw new Error(`timed out waiting for ${kind} message`);
}

function firstURL(text: string): string {
  const match = text.match(/https?:\/\/\S+/);
  assert.ok(match, 'mail contains an auth URL');
  return match[0];
}

function cookieFrom(response: Response): string {
  const setCookie = response.headers.get('set-cookie');
  assert.ok(setCookie, 'sign-in returns a session cookie');
  return setCookie.split(';', 1)[0];
}

test(
  'email verification, password change audit, and reset tokens are enforced end to end',
  { skip: !databaseURL, timeout: 30_000 },
  async (t) => {
    const runtime = await loadRuntime();
    t.after(async () => {
      await runtime.authPool.end();
      await runtime.domainPool.end();
    });
    const nonce = crypto.randomUUID();
    const email = `auth-${nonce}@example.test`;
    const originalPassword = 'original-pass-123';
    const changedPassword = 'changed-pass-456';
    const unauditedPassword = 'changed-but-unaudited-789';
    const resetPassword = 'reset-pass-012';
    const startMessageCount = runtime.mail.sent?.length ?? 0;

    const signup = await runtime.POST(
      jsonRequest('/sign-up/email', {
        name: 'Auth Integration',
        email,
        password: originalPassword,
        callbackURL: '/login?verified=1',
      }),
    );
    assert.equal(signup.status, 200);
    assert.equal(signup.headers.has('set-cookie'), false);

    const duplicateSignup = await runtime.POST(
      jsonRequest('/sign-up/email', {
        name: 'Auth Integration',
        email,
        password: originalPassword,
        callbackURL: '/login?verified=1',
      }),
    );
    assert.equal(duplicateSignup.status, 200);
    assert.equal(duplicateSignup.headers.has('set-cookie'), false);

    const verificationText = await waitForMessage(
      runtime,
      'email_verification',
      email,
      startMessageCount,
    );
    const verificationURL = firstURL(verificationText);

    const unverifiedLogin = await runtime.POST(
      jsonRequest('/sign-in/email', {
        email,
        password: originalPassword,
      }),
    );
    assert.equal(unverifiedLogin.status, 403);
    const loginFailureAudit = await runtime.domainPool.query<{
      actor_id: string | null;
      target: string | null;
      metadata: { status?: number };
    }>(
      `SELECT actor_id, target, metadata
       FROM public.audit_logs
       WHERE action = 'auth.login.failure'
       ORDER BY created_at DESC
       LIMIT 1`,
    );
    assert.equal(loginFailureAudit.rows[0].actor_id, null);
    assert.equal(loginFailureAudit.rows[0].target, null);
    assert.equal(loginFailureAudit.rows[0].metadata.status, 403);

    const verified = await runtime.GET(
      new Request(verificationURL, { redirect: 'manual' }),
    );
    assert.equal(verified.status, 302);
    assert.match(verified.headers.get('location') ?? '', /verified=1/);

    const replay = await runtime.GET(
      new Request(verificationURL, { redirect: 'manual' }),
    );
    assert.equal(replay.status, 302);
    assert.match(replay.headers.get('location') ?? '', /error=INVALID_TOKEN/);

    const { createEmailVerificationToken } = await import('better-auth/api');
    const expiredToken = await createEmailVerificationToken(
      process.env.BETTER_AUTH_SECRET!,
      email,
      undefined,
      -1,
    );
    const expired = await runtime.GET(
      new Request(
        `http://localhost:3000/api/auth/verify-email?token=${expiredToken}&callbackURL=${encodeURIComponent('/login')}`,
        { redirect: 'manual' },
      ),
    );
    assert.equal(expired.status, 302);
    assert.match(expired.headers.get('location') ?? '', /error=TOKEN_EXPIRED/);

    const login = await runtime.POST(
      jsonRequest('/sign-in/email', {
        email,
        password: originalPassword,
      }),
    );
    assert.equal(login.status, 200);
    const cookie = cookieFrom(login);

    const changed = await runtime.POST(
      jsonRequest(
        '/change-password',
        {
          currentPassword: originalPassword,
          newPassword: changedPassword,
          revokeOtherSessions: true,
        },
        cookie,
      ),
    );
    assert.equal(changed.status, 200);
    const passwordAudit = await runtime.domainPool.query<{ count: string }>(
      `SELECT count(*)::text AS count
       FROM public.audit_logs
       WHERE actor_id = (SELECT id FROM auth."user" WHERE email = $1)
         AND action = 'auth.password.change'`,
      [email],
    );
    assert.equal(passwordAudit.rows[0].count, '1');

    const changedCookie = cookieFrom(changed);
    await runtime.domainPool.query(`
      CREATE OR REPLACE FUNCTION public.test_fail_auth_audit()
      RETURNS trigger
      LANGUAGE plpgsql
      AS $$
      BEGIN
        RAISE EXCEPTION 'forced auth audit failure';
      END;
      $$;
      DROP TRIGGER IF EXISTS test_fail_auth_audit ON public.audit_logs;
      CREATE TRIGGER test_fail_auth_audit
      BEFORE INSERT ON public.audit_logs
      FOR EACH ROW EXECUTE FUNCTION public.test_fail_auth_audit()`);
    try {
      const auditFailure = await runtime.POST(
        jsonRequest(
          '/change-password',
          {
            currentPassword: changedPassword,
            newPassword: unauditedPassword,
            revokeOtherSessions: true,
          },
          changedCookie,
        ),
      );
      assert.equal(auditFailure.status, 500);
      const failureBody = (await auditFailure.json()) as {
        error?: { code?: string };
      };
      assert.equal(failureBody.error?.code, 'AUDIT_FAILED');
    } finally {
      await runtime.domainPool.query(`
        DROP TRIGGER IF EXISTS test_fail_auth_audit ON public.audit_logs;
        DROP FUNCTION IF EXISTS public.test_fail_auth_audit()`);
    }

    const failedCredentialLogin = await runtime.POST(
      jsonRequest('/sign-in/email', {
        email,
        password: changedPassword,
      }),
    );
    assert.equal(failedCredentialLogin.status, 401);
    const currentCredentialLogin = await runtime.POST(
      jsonRequest('/sign-in/email', {
        email,
        password: unauditedPassword,
      }),
    );
    assert.equal(currentCredentialLogin.status, 200);

    const resetMessageStart = runtime.mail.sent?.length ?? 0;
    const resetRequest = await runtime.POST(
      jsonRequest('/request-password-reset', {
        email,
        redirectTo: '/reset-password',
      }),
    );
    assert.equal(resetRequest.status, 200);
    const resetText = await waitForMessage(
      runtime,
      'password_reset',
      email,
      resetMessageStart,
    );
    const resetURL = new URL(firstURL(resetText));
    const resetToken = resetURL.pathname.split('/').at(-1);
    assert.ok(resetToken);

    const reset = await runtime.POST(
      jsonRequest('/reset-password', {
        token: resetToken,
        newPassword: resetPassword,
      }),
    );
    assert.equal(reset.status, 200);
    const reusedReset = await runtime.POST(
      jsonRequest('/reset-password', {
        token: resetToken,
        newPassword: 'should-not-work-345',
      }),
    );
    assert.equal(reusedReset.status, 400);

    const oldAfterReset = await runtime.POST(
      jsonRequest('/sign-in/email', {
        email,
        password: unauditedPassword,
      }),
    );
    assert.equal(oldAfterReset.status, 401);
    const newAfterReset = await runtime.POST(
      jsonRequest('/sign-in/email', {
        email,
        password: resetPassword,
      }),
    );
    assert.equal(newAfterReset.status, 200);

  },
);
