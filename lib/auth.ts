import { betterAuth } from 'better-auth';
import { Pool } from 'pg';

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
});
