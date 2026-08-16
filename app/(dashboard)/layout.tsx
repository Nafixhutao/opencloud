import { PanelLeft } from 'lucide-react';
import { redirect } from 'next/navigation';
import type { ReactNode } from 'react';

import {
  AnimatedSidebarInset,
  AnimatedSidebarProvider,
  AnimatedSidebarTrigger,
} from '@/components/motion/animated-sidebar';
import { Sidebar } from '@/components/navigation/sidebar';
import { QueryProvider } from '@/components/providers/query-provider';
import { memberships } from '@/lib/auth';
import { getSession } from '@/lib/session';

export default async function DashboardLayout({
  children,
}: Readonly<{ children: ReactNode }>) {
  const session = await getSession();
  if (!session) {
    redirect('/login');
  }

  let isAdmin = false;
  try {
    const membership = await memberships.getByUserId(session.user.id);
    isAdmin = membership?.role === 'admin';
  } catch {
    isAdmin = false;
  }

  return (
    <AnimatedSidebarProvider className="h-svh overflow-hidden">
      <Sidebar email={session.user.email} isAdmin={isAdmin} />
      <AnimatedSidebarInset className="min-h-0">
        <header className="flex h-12 shrink-0 items-center gap-3 border-b border-border px-4">
          <AnimatedSidebarTrigger
            aria-label="Toggle sidebar"
            className="-ml-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            <PanelLeft aria-hidden="true" className="size-4" />
          </AnimatedSidebarTrigger>
          <span className="text-xs font-medium tracking-wide text-muted-foreground">
            OpenCloud
          </span>
        </header>
        <div className="min-h-0 flex-1 overflow-y-auto">
          <div className="mx-auto w-full max-w-[1200px] px-4 py-6 sm:px-6 lg:px-8">
            <QueryProvider>{children}</QueryProvider>
          </div>
        </div>
      </AnimatedSidebarInset>
    </AnimatedSidebarProvider>
  );
}
