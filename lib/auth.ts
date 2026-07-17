import { betterAuth } from 'better-auth';
import { APIError, createAuthMiddleware } from 'better-auth/api';
import { jwt } from 'better-auth/plugins';
import { Pool } from 'pg';

import { socialProviders } from '@/lib/social-providers';

const databaseUrl = process.env.DATABASE_URL;
const secret = process.env.BETTER_AUTH_SECRET;
const baseURL = process.env.BETTER_AUTH_URL;

if (!databaseUrl || !secret || !baseURL) {
  throw new Error('DATABASE_URL, BETTER_AUTH_SECRET, and BETTER_AUTH_URL are required');
}

export const authPool = new Pool({
  connectionString: databaseUrl,
  options: '-c search_path=auth',
});

export const auth = betterAuth({
  database: authPool,
  secret,
  baseURL,
  emailAndPassword: { enabled: true },
  socialProviders,
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
  },
  // JWKS at /api/auth/jwks, session JWTs via GET /api/auth/token (ADR 0006).
  // Defaults: EdDSA/Ed25519 keys, iss/aud = baseURL, matching the Go middleware
  // and .env.example's AUTH_JWKS_URL/AUTH_ISSUER/AUTH_AUDIENCE values.
  plugins: [jwt()],
});
