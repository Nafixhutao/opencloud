import { Pool } from 'pg';

import { createIdentity } from './identity';
import type { Auth } from './identity';
import { getMailAdapter } from './mail';
import { createMembershipStore, type MembershipStore } from './membership';

/**
 * Composition root — wires the identity module (ADR 0006: better-auth in the
 * BFF) with the membership domain store and mail adapter.
 *
 * The throw for missing env vars lives behind the lazy <code>getRuntime()</code>
 * factory, NOT at module load. This lets <code>next build</code> statically
 * analyse routes that import this module without triggering the DB/env
 * validation — the CI build failure that motivated this split.
 *
 * Consumers that need the live instance call <code>getRuntime()</code> at
 * request time. Re-exports (<code>auth</code>, <code>memberships</code>,
 * <code>mail</code>) are lazy getters for backward compatibility with existing
 * route handlers and pages.
 */

type AuthRuntime = {
  auth: Auth['auth'];
  authPool: Pool;
  domainPool: Pool;
  memberships: MembershipStore;
  mail: ReturnType<typeof getMailAdapter>;
};

let runtime: AuthRuntime | null = null;

function getRuntime(): AuthRuntime {
  if (runtime) {
    return runtime;
  }

  const databaseUrl = process.env.DATABASE_URL;
  const secret = process.env.BETTER_AUTH_SECRET;
  const baseURL = process.env.BETTER_AUTH_URL;

  if (!databaseUrl || !secret || !baseURL) {
    throw new Error('DATABASE_URL, BETTER_AUTH_SECRET, and BETTER_AUTH_URL are required');
  }

  const domainPool = new Pool({
    connectionString: databaseUrl,
    options: '-c search_path=public',
  });

  const memberships = createMembershipStore(domainPool);
  const mail = getMailAdapter();
  const { auth, authPool } = createIdentity({
    databaseUrl,
    secret,
    baseURL,
    memberships,
    mail,
  });

  runtime = { auth, authPool, domainPool, memberships, mail };
  return runtime;
}

/** Lazy getter — evaluates <code>getRuntime()</code> on first access. */
export const auth: Auth['auth'] = new Proxy({} as Auth['auth'], {
  get(_target, prop, receiver) {
    return Reflect.get(getRuntime().auth, prop, receiver);
  },
}) as Auth['auth'];

export const memberships: MembershipStore = new Proxy(
  {} as MembershipStore,
  {
    get(_target, prop, receiver) {
      return Reflect.get(getRuntime().memberships, prop, receiver);
    },
  },
) as MembershipStore;

export const mail = new Proxy({} as ReturnType<typeof getMailAdapter>, {
  get(_target, prop, receiver) {
    return Reflect.get(getRuntime().mail, prop, receiver);
  },
}) as ReturnType<typeof getMailAdapter>;

/** Pool singletons — lazy via <code>getRuntime()</code>. */
function lazyPool(key: 'authPool' | 'domainPool'): Pool {
  return new Proxy({} as Pool, {
    get(_target, prop, receiver) {
      return Reflect.get(getRuntime()[key], prop, receiver);
    },
  }) as Pool;
}

export const authPool = lazyPool('authPool');
export const domainPool = lazyPool('domainPool');

export type Session = Auth['auth']['$Infer']['Session'];
