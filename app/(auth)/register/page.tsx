import type { Metadata } from 'next';

import { RegisterForm } from '@/components/auth/register-form';
import { enabledSocialProviders } from '@/lib/social-providers';

// oxlint-disable-next-line react/only-export-components -- Next.js reads this page export.
export const metadata: Metadata = {
  title: 'Register · OpenCloud',
  description: 'Create an OpenCloud account.',
};

type RegisterPageProps = {
  searchParams: Promise<{ error?: string | string[] }>;
};

export default async function RegisterPage({ searchParams }: RegisterPageProps) {
  const { error } = await searchParams;

  return (
    <RegisterForm
      enabledSocialProviders={enabledSocialProviders}
      initialError={
        error ? 'Social sign-up could not be completed. Try again or use email.' : null
      }
    />
  );
}
