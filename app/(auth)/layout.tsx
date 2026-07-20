import { redirect } from 'next/navigation';
import type { ReactNode } from 'react';

import { AuthSpotlight } from '@/components/auth/auth-spotlight';
import { BrandLogo } from '@/components/brand-logo';
import { getSession } from '@/lib/session';

export default async function AuthLayout({ children }: Readonly<{ children: ReactNode }>) {
  const session = await getSession();
  if (session) {
    redirect('/dashboard');
  }

  return (
    <main className="min-h-svh bg-background">
      <a
        href="#auth-form"
        className="sr-only rounded-sm bg-primary px-3 py-2 text-sm font-medium text-primary-foreground focus:not-sr-only focus:fixed focus:left-4 focus:top-4"
      >
        Skip to account form
      </a>
      <div className="mx-auto grid min-h-svh w-full max-w-[1200px] border-x border-border lg:grid-cols-[5fr_7fr]">
        <section
          id="auth-form"
          className="flex min-h-svh flex-col bg-background px-6 py-6 sm:px-10 sm:py-8 lg:px-12"
        >
          <header className="flex items-center justify-between gap-4">
            <BrandLogo priority />
            <span className="label-meta hidden text-muted-foreground sm:inline">
              Control Plane
            </span>
          </header>
          <div className="flex flex-1 items-center py-16 lg:py-20">
            <div className="mx-auto w-full max-w-[25rem]">{children}</div>
          </div>
          <p className="label-meta text-muted-foreground">
            Encrypted Session · Managed Access
          </p>
        </section>

        <AuthSpotlight />
      </div>
    </main>
  );
}
