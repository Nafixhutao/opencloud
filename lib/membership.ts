import type { Pool, PoolClient, QueryResult } from 'pg';

/**
 * Domain membership helpers against public.account_memberships / public.accounts.
 * Identity lives in auth.*; tenancy lives in public.* (ADR 0006, DATABASE.md).
 *
 * Role is always server-assigned — never accepted from client input.
 */

export type MembershipRole = 'customer' | 'admin';
export type MembershipStatus = 'active' | 'suspended' | 'disabled';

export type Membership = {
  id: string;
  account_id: string;
  user_id: string;
  role: MembershipRole;
  status: MembershipStatus;
};

export type AuditEntry = {
  accountId?: string | null;
  actorId?: string | null;
  action: string;
  target?: string | null;
  metadata?: Record<string, unknown>;
};

type Queryable = Pick<Pool, 'query'> | Pick<PoolClient, 'query'>;

async function appendAuditWith(client: Queryable, entry: AuditEntry): Promise<void> {
  await client.query(
    `INSERT INTO public.audit_logs (account_id, actor_id, action, target, metadata)
     VALUES ($1, $2, $3, $4, $5::jsonb)`,
    [
      entry.accountId ?? null,
      entry.actorId ?? null,
      entry.action,
      entry.target ?? null,
      JSON.stringify(entry.metadata ?? {}),
    ],
  );
}

/** Pool must be able to read/write public.* (default search_path). */
export function createMembershipStore(pool: Pool) {
  async function getByUserId(userId: string): Promise<Membership | null> {
    const { rows } = await pool.query<Membership>(
      `SELECT id, account_id, user_id, role, status
       FROM public.account_memberships
       WHERE user_id = $1
       LIMIT 1`,
      [userId],
    );
    return rows[0] ?? null;
  }

  async function ensure(
    userId: string,
    accountName: string,
    creationAudit?: AuditEntry,
  ): Promise<Membership> {
    const existing = await getByUserId(userId);
    if (existing) {
      return existing;
    }

    const name = accountName.trim() || 'Workspace';
    const client = await pool.connect();
    try {
      await client.query('BEGIN');
      await client.query(
        `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
        [userId],
      );

      const again = await client.query<Membership>(
        `SELECT id, account_id, user_id, role, status
         FROM public.account_memberships
         WHERE user_id = $1
         LIMIT 1`,
        [userId],
      );
      if (again.rows[0]) {
        await client.query('COMMIT');
        return again.rows[0];
      }

      const account = await client.query<{ id: string }>(
        `INSERT INTO public.accounts (name, status)
         VALUES ($1, 'active')
         RETURNING id`,
        [name],
      );
      const accountId = account.rows[0].id;
      const inserted = await client.query<Membership>(
        `INSERT INTO public.account_memberships
           (account_id, user_id, role, status)
         VALUES ($1, $2, 'customer', 'active')
         ON CONFLICT (user_id) DO NOTHING
         RETURNING id, account_id, user_id, role, status`,
        [accountId, userId],
      );

      let membership = inserted.rows[0];
      if (!membership) {
        await client.query(`DELETE FROM public.accounts WHERE id = $1`, [accountId]);
        const winner = await client.query<Membership>(
          `SELECT id, account_id, user_id, role, status
           FROM public.account_memberships
           WHERE user_id = $1
           LIMIT 1`,
          [userId],
        );
        membership = winner.rows[0];
        if (!membership) {
          throw new Error('membership conflict had no committed winner');
        }
      } else if (creationAudit) {
        await appendAuditWith(client, {
          ...creationAudit,
          accountId: membership.account_id,
          actorId: creationAudit.actorId ?? userId,
          target: creationAudit.target ?? userId,
        });
      }

      await client.query('COMMIT');
      return membership;
    } catch (error) {
      try {
        await client.query('ROLLBACK');
      } catch {
        // Preserve the original error.
      }
      throw error;
    } finally {
      client.release();
    }
  }

  return {
    getByUserId,

    async ensureForUser(userId: string, accountName: string): Promise<Membership> {
      return ensure(userId, accountName);
    },

    async ensureForUserWithAudit(
      userId: string,
      accountName: string,
      audit: AuditEntry,
    ): Promise<Membership> {
      return ensure(userId, accountName, audit);
    },

    async appendAudit(entry: AuditEntry): Promise<void> {
      await appendAuditWith(pool, entry);
    },

    async claimEmailVerificationToken(
      tokenHash: Buffer,
      expiresAt: Date,
    ): Promise<boolean> {
      const result: QueryResult = await pool.query(
        `INSERT INTO public.auth_token_consumptions (token_hash, kind, expires_at)
         VALUES ($1, 'email_verification', $2)
         ON CONFLICT (token_hash) DO NOTHING`,
        [tokenHash, expiresAt],
      );
      return result.rowCount === 1;
    },

    async releaseEmailVerificationToken(tokenHash: Buffer): Promise<void> {
      await pool.query(
        `DELETE FROM public.auth_token_consumptions
         WHERE token_hash = $1 AND kind = 'email_verification'`,
        [tokenHash],
      );
    },

    /**
     * Purge expired single-use token claims. Called opportunistically after
     * successful verification so the table cannot grow unboundedly; rows left
     * behind by abandoned links are swept by later runs.
     */
    async pruneExpiredTokenConsumptions(): Promise<void> {
      await pool.query(
        `DELETE FROM public.auth_token_consumptions WHERE expires_at < now()`,
      );
    },
  };
}

export type MembershipStore = ReturnType<typeof createMembershipStore>;
