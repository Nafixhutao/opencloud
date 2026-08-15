import { getMigrations } from 'better-auth/db/migration';

import { auth, authPool } from '../lib/auth';

try {
  const { runMigrations } = await getMigrations(auth.options);
  await runMigrations();
} catch (error) {
  console.error(error);
  process.exit(1);
} finally {
  authPool.end().catch(() => {});
}
process.exit(0);
