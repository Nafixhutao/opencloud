import type { Pool } from 'pg';

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

export type AccountRow = {
  id: string;
  name: string;
  status: string;
};

/** Pool must be able to read/write public.* (default search_path). */
export function createMembershipStore(pool: Pool) {
  return {
    async getByUserId(userId: string): Promise<Membership | null> {
      const { rows } = await pool.query<Membership>(
        `SELECT id, account_id, user_id, role, status
         FROM public.account_memberships
         WHERE user_id = $1
         LIMIT 1`,
        [userId],
      );
      return rows[0] ?? null;
    },

    /**
     * Create tenant account + customer membership for a new user.
     * Safe under concurrency: UNIQUE(user_id) + re-read on conflict.
     */
    async ensureForUser(userId: string, accountName: string): Promise<Membership> {
      const existing = await this.getByUserId(userId);
      if (existing) {
        return existing;
      }

      const name = accountName.trim() || 'Workspace';
      const client = await pool.connect();
      try {
        await client.query('BEGIN');

        const again = await client.query<Membership>(
          `SELECT id, account_id, user_id, role, status
           FROM public.account_memberships
           WHERE user_id = $1
           LIMIT 1
           FOR UPDATE`,
          [userId],
        );
        if (again.rows[0]) {
          await client.query('COMMIT');
          return again.rows[0];
        }

        const acct = await client.query<AccountRow>(
          `INSERT INTO public.accounts (name, status)
           VALUES ($1, 'active')
           RETURNING id, name, status`,
          [name],
        );
        const accountId = acct.rows[0].id;

        try {
          const mem = await client.query<Membership>(
            `INSERT INTO public.account_memberships
               (account_id, user_id, role, status)
             VALUES ($1, $2, 'customer', 'active')
             RETURNING id, account_id, user_id, role, status`,
            [accountId, userId],
          );
          await client.query('COMMIT');
          return mem.rows[0];
        } catch (err: unknown) {
          // Concurrent insert on unique user_id — re-read winner.
          await client.query('ROLLBACK');
          const winner = await this.getByUserId(userId);
          if (winner) {
            return winner;
          }
          throw err;
        }
      } catch (err) {
        try {
          await client.query('ROLLBACK');
        } catch {
          // ignore
        }
        throw err;
      } finally {
        client.release();
      }
    },

    async appendAudit(entry: {
      accountId?: string | null;
      actorId?: string | null;
      action: string;
      target?: string | null;
      metadata?: Record<string, unknown>;
    }): Promise<void> {
      await pool.query(
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
    },
  };
}

export type MembershipStore = ReturnType<typeof createMembershipStore>;
