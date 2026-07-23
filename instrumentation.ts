/**
 * Validate runtime-only integrations before the dashboard starts accepting
 * traffic. In production, getMailAdapter rejects non-delivery providers and
 * incomplete SMTP credentials.
 */
export async function register(): Promise<void> {
  if (process.env.NEXT_RUNTIME !== 'nodejs') {
    return;
  }
  const { validateProductionMailAtStartup } = await import('./instrumentation-node');
  await validateProductionMailAtStartup();
}
