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
  searchParams: Promise<{ error?: string | string[] }>;
};

export default async function LoginPage({ searchParams }: LoginPageProps) {
  const { error } = await searchParams;

  return (
    <LoginForm
      enabledSocialProviders={enabledSocialProviders}
      initialError={getAuthCallbackError(error, 'login')}
    />
  );
}
