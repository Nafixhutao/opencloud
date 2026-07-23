import type { Metadata } from 'next';

import { ResetPasswordForm } from '@/components/auth/reset-password-form';

// oxlint-disable-next-line react/only-export-components -- Next.js page metadata.
export const metadata: Metadata = {
  title: 'Reset password',
  description: 'Set a new password with a one-time reset token.',
};

type ResetPasswordPageProps = {
  searchParams: Promise<{ token?: string | string[]; error?: string | string[] }>;
};

export default async function ResetPasswordPage({ searchParams }: ResetPasswordPageProps) {
  const params = await searchParams;
  const tokenRaw = params.token;
  const token = Array.isArray(tokenRaw) ? tokenRaw[0] : tokenRaw ?? null;
  const errorRaw = params.error;
  const error = Array.isArray(errorRaw) ? errorRaw[0] : errorRaw;
  const initialError =
    error === 'INVALID_TOKEN'
      ? 'This reset link is invalid or has expired. Request a new one.'
      : null;

  return <ResetPasswordForm token={token} initialError={initialError} />;
}
