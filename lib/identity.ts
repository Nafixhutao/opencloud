import { betterAuth } from 'better-auth';
import { APIError, createAuthMiddleware } from 'better-auth/api';
import { jwt } from 'better-auth/plugins';
import { Pool } from 'pg';

import type { MembershipStore } from './membership';
import type { MailAdapter } from './mail';
import { socialProviders } from './social-providers';

/**
 * Identity module — configures better-auth (ADR 0006: BFF owns identity).
 *
 * This module is deep: one interface (<code>createIdentity</code>), one
 * implementation that owns better-auth configuration, hooks, and the JWT
 * payload. It does NOT import membership or mail directly — they are injected,
 * so the identity/domain seam (ADR 0006) is real and the module is testable.
 *
 * No module-load side effects: importing this file never touches env vars,
 * databases, or SMTP. The throw lives behind the factory, so <code>next build</code>
 * can statically analyse any route that imports this module without breaking.
 */

export type IdentityConfig = {
  databaseUrl: string;
  secret: string;
  baseURL: string;
  memberships: MembershipStore;
  mail: Pick<MailAdapter, 'send'>;
};

export type Auth = ReturnType<typeof createIdentity>;

/** Lazily create the better-auth instance. Throws if config is missing. */
export function createIdentity(config: IdentityConfig) {
  const { databaseUrl, secret, baseURL, memberships, mail } = config;

  const authPool = new Pool({
    connectionString: databaseUrl,
    options: '-c search_path=auth',
  });

  const auth = betterAuth({
    database: authPool,
    secret,
    baseURL,
    emailVerification: {
      expiresIn: 3600,
      sendOnSignUp: true,
      sendOnSignIn: true,
      autoSignInAfterVerification: false,
      sendVerificationEmail: async ({ user, url }) => {
        await mail.send({
          to: user.email,
          subject: 'Verify your OpenCloud email',
          text: `Verify your email using this one-time link (expires in 1 hour):\n\n${url}\n\nIf you did not create this account, ignore this email.`,
          tags: { kind: 'email_verification', user_id: user.id },
        });
      },
      afterEmailVerification: async (user) => {
        await memberships.appendAudit({
          actorId: user.id,
          action: 'auth.email.verify',
          target: user.id,
        });
      },
    },
    emailAndPassword: {
      enabled: true,
      requireEmailVerification: true,
      autoSignIn: false,
      minPasswordLength: 8,
      maxPasswordLength: 128,
      resetPasswordTokenExpiresIn: 3600,
      revokeSessionsOnPasswordReset: true,
      sendResetPassword: async ({ user, url }) => {
        await mail.send({
          to: user.email,
          subject: 'Reset your OpenCloud password',
          text: `Reset your password using this one-time link (expires in 1 hour):\n\n${url}\n\nIf you did not request this, ignore this email.`,
          tags: { kind: 'password_reset', user_id: user.id },
        });
        await memberships.appendAudit({
          actorId: user.id,
          action: 'auth.password_reset.request',
          target: user.id,
          metadata: { email_domain: user.email.split('@')[1] ?? '' },
        });
      },
      onPasswordReset: async ({ user }) => {
        await memberships.appendAudit({
          actorId: user.id,
          action: 'auth.password_reset.complete',
          target: user.id,
        });
      },
    },
    socialProviders,
    rateLimit: {
      enabled: true,
      window: 60,
      max: 100,
      // Database-backed counters survive restarts and are shared across
      // instances — memory storage silently multiplies every limit below by
      // the replica count and resets on each deploy.
      storage: 'database',
      customRules: {
        '/sign-in/email': { window: 60, max: 10 },
        '/sign-up/email': { window: 60, max: 5 },
        '/request-password-reset': { window: 60, max: 3 },
        '/reset-password': { window: 60, max: 5 },
        '/change-password': { window: 60, max: 5 },
        '/send-verification-email': { window: 60, max: 3 },
      },
    },
    hooks: {
      before: createAuthMiddleware(async (ctx) => {
        if (ctx.path !== '/sign-up/email') {
          return;
        }

        const name = typeof ctx.body?.name === 'string' ? ctx.body.name.trim() : '';
        if (!name) {
          throw APIError.fromStatus('BAD_REQUEST', { message: 'Name is required' });
        }
        if (name.length > 100) {
          throw APIError.fromStatus('BAD_REQUEST', {
            message: 'Name must be at most 100 characters',
          });
        }
        ctx.body.name = name;
      }),
      after: createAuthMiddleware(async (ctx) => {
        if (ctx.path === '/sign-in/email' && ctx.context.newSession?.user) {
          const user = ctx.context.newSession.user;
          await memberships.appendAudit({
            actorId: user.id,
            action: 'auth.login.success',
            target: user.id,
          });
        }
      }),
    },
    databaseHooks: {
      user: {
        create: {
          after: async (user) => {
            // Membership creation and its audit row commit together. Admin remains
            // an explicit bootstrap-only platform role.
            await memberships.ensureForUserWithAudit(user.id, user.name || 'Workspace', {
              actorId: user.id,
              action: 'account.membership.ensure',
              target: user.id,
              metadata: { role: 'customer', source: 'user.create' },
            });
          },
        },
      },
    },
    plugins: [
      jwt({
        jwt: {
          definePayload: async (session) => {
            const user = session.user;
            const membership = await memberships.ensureForUser(
              user.id,
              user.name || 'Workspace',
            );
            if (membership.status !== 'active') {
              throw APIError.fromStatus('FORBIDDEN', {
                message: 'Account is not active',
              });
            }
            return {
              account_id: membership.account_id,
              role: membership.role,
              email: user.email,
              name: user.name,
            };
          },
        },
      }),
    ],
  });

  return { auth, authPool };
}
