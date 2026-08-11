import { redirect } from 'next/navigation';
import type { ReactNode } from 'react';

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
    <div className="flex h-screen overflow-hidden bg-zinc-900">
      <Sidebar email={session.user.email} isAdmin={isAdmin} />
      <main className="flex-1 overflow-y-auto bg-zinc-900">
        <div className="mx-auto w-full max-w-[1200px] px-4 py-6 sm:px-6 lg:px-8">
          <QueryProvider>{children}</QueryProvider>
        </div>
      </main>
    </div>
  );
}
