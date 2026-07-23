import { betterAuth } from 'better-auth';
import { APIError, createAuthMiddleware } from 'better-auth/api';
import { jwt } from 'better-auth/plugins';
import { Pool } from 'pg';

import { getMailAdapter } from './mail';
import { createMembershipStore } from './membership';
import { socialProviders } from './social-providers';

const databaseUrl = process.env.DATABASE_URL;
const secret = process.env.BETTER_AUTH_SECRET;
const baseURL = process.env.BETTER_AUTH_URL;

if (!databaseUrl || !secret || !baseURL) {
  throw new Error('DATABASE_URL, BETTER_AUTH_SECRET, and BETTER_AUTH_URL are required');
}

/** Auth schema pool — better-auth tables (search_path=auth). */
export const authPool = new Pool({
  connectionString: databaseUrl,
  options: '-c search_path=auth',
});

/** Domain pool — public.accounts / memberships / audit_logs. */
export const domainPool = new Pool({
  connectionString: databaseUrl,
  options: '-c search_path=public',
});

export const memberships = createMembershipStore(domainPool);

const mail = getMailAdapter();

export const auth = betterAuth({
  database: authPool,
  secret,
  baseURL,
  emailAndPassword: {
    enabled: true,
    minPasswordLength: 8,
    maxPasswordLength: 128,
    resetPasswordTokenExpiresIn: 3600,
    revokeSessionsOnPasswordReset: true,
    sendResetPassword: async ({ user, url }) => {
      // Do not await to reduce timing side-channels (better-auth docs).
      // Never log `url` or token — they are single-use secrets.
      void mail
        .send({
          to: user.email,
          subject: 'Reset your Cevra password',
          text: `Reset your password using this one-time link (expires in 1 hour):\n\n${url}\n\nIf you did not request this, ignore this email.`,
          tags: { kind: 'password_reset', user_id: user.id },
        })
        .catch((err: unknown) => {
          console.error('[auth] sendResetPassword failed', {
            user_id: user.id,
            error: err instanceof Error ? err.message : 'unknown',
          });
        });

      void memberships
        .appendAudit({
          actorId: user.id,
          action: 'auth.password_reset.request',
          target: user.id,
          metadata: { email_domain: user.email.split('@')[1] ?? '' },
        })
        .catch(() => {
          /* audit best-effort */
        });
    },
    onPasswordReset: async ({ user }) => {
      void memberships
        .appendAudit({
          actorId: user.id,
          action: 'auth.password_reset.complete',
          target: user.id,
        })
        .catch(() => {
          /* audit best-effort */
        });
    },
  },
  socialProviders,
  // Rate limiting is on by default in production; force-enable in all envs for
  // auth abuse protection (SECURITY §9). Memory storage is fine for single-node
  // BFF; Redis secondary storage can land later without API changes.
  rateLimit: {
    enabled: true,
    window: 60,
    max: 100,
    storage: 'memory',
    customRules: {
      '/sign-in/email': { window: 60, max: 10 },
      '/sign-up/email': { window: 60, max: 5 },
      '/request-password-reset': { window: 60, max: 3 },
      '/reset-password': { window: 60, max: 5 },
      '/change-password': { window: 60, max: 5 },
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
      // Audit successful credential sign-in (no password/token in metadata).
      if (ctx.path === '/sign-in/email' && ctx.context.newSession?.user) {
        const user = ctx.context.newSession.user;
        void memberships
          .appendAudit({
            actorId: user.id,
            action: 'auth.login.success',
            target: user.id,
          })
          .catch(() => {
            /* best-effort */
          });
      }
    }),
  },
  databaseHooks: {
    user: {
      create: {
        after: async (user) => {
          // Signup (email or social) always gets a tenant membership as customer.
          // Role admin is never assigned here — bootstrap-admin is the only path.
          try {
            const membership = await memberships.ensureForUser(user.id, user.name || 'Workspace');
            await memberships.appendAudit({
              accountId: membership.account_id,
              actorId: user.id,
              action: 'account.membership.ensure',
              target: user.id,
              metadata: { role: membership.role, source: 'user.create' },
            });
          } catch (err) {
            // After-hooks run post-commit; failure must not leave auth user without
            // a recovery path — GetMe / JWT definePayload call ensure again.
            console.error('[auth] ensure membership after user.create failed', {
              user_id: user.id,
              error: err instanceof Error ? err.message : 'unknown',
            });
          }
        },
      },
    },
  },
  // JWKS at /api/auth/jwks; session JWTs via GET /api/auth/token (ADR 0006).
  // Custom claims account_id + role come only from server-side membership rows.
  plugins: [
    jwt({
      jwt: {
        // definePayload receives the full session ({ user, session }) per jwt plugin.
        definePayload: async (session) => {
          const user = session.user;
          const membership = await memberships.ensureForUser(
            user.id,
            user.name || 'Workspace',
          );
          if (membership.status !== 'active') {
            // Refuse to mint a usable tenant token for suspended/disabled users.
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
        // iss/aud default to baseURL origin — match AUTH_ISSUER/AUTH_AUDIENCE.
      },
    }),
  ],
});

export type Session = typeof auth.$Infer.Session;
