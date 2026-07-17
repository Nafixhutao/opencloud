import { redirect } from 'next/navigation';
import type { ReactNode } from 'react';

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
  return <>{children}</>;
}
