import assert from 'node:assert/strict';
import test from 'node:test';

import { Pool } from 'pg';

import { createMembershipStore } from './membership.ts';

const databaseURL = process.env.DATABASE_URL;

test(
  'concurrent membership callers converge on one account without orphans',
  { skip: !databaseURL },
  async () => {
    const pool = new Pool({ connectionString: databaseURL });
    const store = createMembershipStore(pool);
    const nonce = crypto.randomUUID();
    const userID = `ts_membership_${nonce}`;
    const accountName = `TS membership race ${nonce}`;
    try {
      const memberships = await Promise.all(
        Array.from({ length: 12 }, () => store.ensureForUser(userID, accountName)),
      );
      assert.equal(new Set(memberships.map((row) => row.id)).size, 1);
      assert.equal(new Set(memberships.map((row) => row.account_id)).size, 1);

      const accounts = await pool.query<{ id: string }>(
        `SELECT id FROM public.accounts WHERE name = $1`,
        [accountName],
      );
      assert.equal(accounts.rowCount, 1);
      assert.equal(accounts.rows[0].id, memberships[0].account_id);
    } finally {
      await pool.end();
    }
  },
);
