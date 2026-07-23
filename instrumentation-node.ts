import { getMailAdapter } from './lib/mail';

export async function validateProductionMailAtStartup(): Promise<void> {
  try {
    getMailAdapter();
  } catch {
    // Next.js logs instrumentation exceptions but can keep the server process
    // alive. Production must not accept traffic without a delivery provider.
    console.error('[startup] production mail configuration is invalid');
    process.exit(1);
  }
}
