'use client';

import type { ReactNode } from 'react';

import { AnimatedSidebarProvider } from '@/components/motion/animated-sidebar';

export const SIDEBAR_STATE_COOKIE = 'sidebar_state';

const ONE_YEAR_SECONDS = 60 * 60 * 24 * 365;

type SidebarShellProps = {
  defaultOpen: boolean;
  children: ReactNode;
};

/**
 * Persists the rail open/closed choice so it survives navigation and reloads.
 * A cookie (not localStorage) so the server layout can render the correct width
 * on first paint and avoid a collapse flash.
 */
export function SidebarShell({ defaultOpen, children }: SidebarShellProps) {
  return (
    <AnimatedSidebarProvider
      defaultOpen={defaultOpen}
      onOpenChange={(open) => {
        document.cookie = `${SIDEBAR_STATE_COOKIE}=${open}; path=/; max-age=${ONE_YEAR_SECONDS}; samesite=lax`;
      }}
      className="h-svh overflow-hidden"
    >
      {children}
    </AnimatedSidebarProvider>
  );
}
