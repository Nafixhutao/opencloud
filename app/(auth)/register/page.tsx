import type { Metadata } from 'next';

import { RegisterForm } from '@/components/auth/register-form';
import { getAuthCallbackError } from '@/lib/auth-errors';
import { enabledSocialProviders } from '@/lib/social-providers';

// oxlint-disable-next-line react/only-export-components -- Next.js reads this page export.
export const metadata: Metadata = {
  title: 'Create account',
  description: 'Create a Cevra control-plane account.',
};

type RegisterPageProps = {
  searchParams: Promise<{ error?: string | string[] }>;
};

export default async function RegisterPage({ searchParams }: RegisterPageProps) {
  const { error } = await searchParams;

  return (
    <RegisterForm
      enabledSocialProviders={enabledSocialProviders}
      initialError={getAuthCallbackError(error, 'register')}
    />
  );
}
