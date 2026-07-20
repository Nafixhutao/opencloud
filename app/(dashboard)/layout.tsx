import { redirect } from 'next/navigation';
import Link from 'next/link';
import type { ReactNode } from 'react';

import { SignOutButton } from '@/components/auth/sign-out-button';
import { BrandLogo } from '@/components/brand-logo';
import { getSession } from '@/lib/session';

// Shell for the authenticated customer area: server-side session guard.
// Full navigation lands with the first real dashboard screens (Phase 1).
export default async function DashboardLayout({
  children,
}: Readonly<{ children: ReactNode }>) {
  const session = await getSession();
  if (!session) {
    redirect('/login');
  }

  return (
    <div className="min-h-svh bg-background">
      <a
        href="#dashboard-content"
        className="sr-only rounded-sm bg-primary px-3 py-2 text-sm font-medium text-primary-foreground focus:not-sr-only focus:fixed focus:left-4 focus:top-4"
      >
        Skip to dashboard
      </a>
      <header className="sticky top-0 z-20 min-h-16 border-b border-border bg-background/95 backdrop-blur-md">
        <div className="mx-auto flex min-h-16 w-full max-w-[1200px] items-center justify-between gap-5 px-6 sm:px-8">
          <div className="flex min-w-0 items-center gap-8">
            <BrandLogo priority className="h-6" />
            <nav aria-label="Dashboard navigation" className="hidden items-center gap-1 sm:flex">
              <Link
                href="/dashboard"
                aria-current="page"
                className="rounded-sm bg-muted px-3 py-2 text-sm font-medium"
              >
                Overview
              </Link>
              <a
                href="#workspace"
                className="rounded-sm px-3 py-2 text-sm text-muted-foreground hover:bg-secondary hover:text-foreground"
              >
                Setup
              </a>
            </nav>
          </div>

          <div className="flex items-center gap-3">
            <p className="hidden max-w-52 truncate text-sm text-muted-foreground md:block">
              {session.user.email}
            </p>
            <SignOutButton />
          </div>
        </div>
      </header>
      {children}
    </div>
  );
}
