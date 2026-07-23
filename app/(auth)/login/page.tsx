import type { Metadata } from 'next';

import { LoginForm } from '@/components/auth/login-form';
import { getAuthCallbackError } from '@/lib/auth-errors';
import { enabledSocialProviders } from '@/lib/social-providers';

// oxlint-disable-next-line react/only-export-components -- Next.js reads this page export.
export const metadata: Metadata = {
  title: 'Sign in',
  description: 'Sign in to the Cevra control plane.',
};

type LoginPageProps = {
  searchParams: Promise<{
    error?: string | string[];
    notice?: string | string[];
    verified?: string | string[];
  }>;
};

export default async function LoginPage({ searchParams }: LoginPageProps) {
  const { error, notice, verified } = await searchParams;
  const noticeValue = Array.isArray(notice) ? notice[0] : notice;
  const verifiedValue = Array.isArray(verified) ? verified[0] : verified;
  const initialNotice =
    verifiedValue === '1'
      ? 'Email verified. You can sign in now.'
      : noticeValue === 'verify-email'
        ? 'Check your email for a one-time verification link before signing in.'
        : null;

  return (
    <LoginForm
      enabledSocialProviders={enabledSocialProviders}
      initialError={getAuthCallbackError(error, 'login')}
      initialNotice={initialNotice}
    />
  );
}
