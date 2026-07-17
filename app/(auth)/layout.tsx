import { redirect } from 'next/navigation';
import type { ReactNode } from 'react';

import { AuthPage } from '@/components/auth-page';
import { getSession } from '@/lib/session';

export default async function AuthLayout({ children }: Readonly<{ children: ReactNode }>) {
  const session = await getSession();
  if (session) {
    redirect('/dashboard');
  }

  return <AuthPage>{children}</AuthPage>;
}
