import type { Metadata } from 'next';

import { LoginForm } from '@/components/auth/login-form';
import { enabledSocialProviders } from '@/lib/social-providers';

// oxlint-disable-next-line react/only-export-components -- Next.js reads this page export.
export const metadata: Metadata = {
  title: 'Log in · OpenCloud',
  description: 'Log in to the OpenCloud dashboard.',
};

type LoginPageProps = {
  searchParams: Promise<{ error?: string | string[] }>;
};

export default async function LoginPage({ searchParams }: LoginPageProps) {
  const { error } = await searchParams;

  return (
    <LoginForm
      enabledSocialProviders={enabledSocialProviders}
      initialError={
        error ? 'Social sign-in could not be completed. Try again or use email.' : null
      }
    />
  );
}
