import { getMigrations } from 'better-auth/db/migration';

import { auth, authPool } from '../lib/auth';

try {
  const { runMigrations } = await getMigrations(auth.options);
  await runMigrations();
} finally {
  await authPool.end();
}
