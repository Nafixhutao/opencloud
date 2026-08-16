/**
 * Operator tool: mark one user's email as verified.
 *
 *   npm run auth:verify-email -- --email user@example.com
 *   npm run auth:verify-email -- --user-id <better-auth user id>
 *
 * Intended for support flows (user lost the verification email) and for
 * stacks where MAIL_PROVIDER=log delivered the link only into server logs.
 * Idempotent: verifying an already-verified user is a success no-op.
 * Exits non-zero when no user matches, or when both flags are given.
 */
import { Pool } from 'pg';

function parseArgs(argv: string[]) {
  const flags: Record<string, string> = {};
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === '--email' || arg === '--user-id') {
      const value = argv[i + 1];
      if (!value) {
        throw new Error(`${arg} requires a value`);
      }
      flags[arg.slice(2)] = value;
      i += 1;
    } else {
      throw new Error(`unknown argument: ${arg}`);
    }
  }
  return flags;
}

function usage(): never {
  console.error('usage: npm run auth:verify-email -- --email <email> | --user-id <id>');
  process.exit(2);
}

const databaseUrl = process.env.DATABASE_URL;
if (!databaseUrl) {
  console.error('DATABASE_URL is required');
  process.exit(1);
}

let flags: Record<string, string>;
try {
  flags = parseArgs(process.argv.slice(2));
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  usage();
}
if (!flags.email === !flags['user-id']) {
  usage();
}

const pool = new Pool({ connectionString: databaseUrl, options: '-c search_path=auth' });
try {
  const where = flags.email ? 'email = $1' : 'id = $1';
  const selector = flags.email ?? flags['user-id'];
  const result = await pool.query(
    `UPDATE "user" SET "emailVerified" = true, "updatedAt" = now()
     WHERE ${where}
       AND "emailVerified" = false
     RETURNING id, email`,
    [selector],
  );
  if (result.rowCount === 0) {
    // Distinguish "no such user" from "already verified" so operators can
    // tell a typo apart from a no-op.
    const existing = await pool.query(
      `SELECT id, email, "emailVerified" FROM "user" WHERE ${where}`,
      [selector],
    );
    if (existing.rowCount === 0) {
      console.error(`no user found for ${flags.email ? 'email' : 'id'} ${selector}`);
      process.exit(1);
    }
    const row = existing.rows[0];
    console.log(`already verified: ${row.email} (${row.id})`);
  } else {
    const row = result.rows[0];
    console.log(`verified: ${row.email} (${row.id})`);
  }
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exit(1);
} finally {
  await pool.end().catch(() => {});
}
