'use client';

import { ChevronsUpDown } from 'lucide-react';

import { WorkspaceAvatar } from '@/planing-ui/sidebar/workspace-avatar';
import { cn } from '@/lib/utils';

type SidebarFooterProps = {
  name: string;
  role: string;
  collapsed: boolean;
};

/**
 * Account row pinned to the bottom of the panel by `mt-auto`, so it stays flush
 * to the bottom edge regardless of menu height. At rail width only the avatar
 * remains, centered.
 */
export function SidebarFooter({ name, role, collapsed }: SidebarFooterProps) {
  return (
    <div
      className={cn(
        'mt-auto shrink-0 border-t border-[var(--ref-divider)]',
        collapsed ? 'flex justify-center px-0 py-3' : 'px-[12px] py-2.5',
      )}
    >
      <button
        type="button"
        aria-label={collapsed ? `${name}, ${role}` : undefined}
        title={collapsed ? `${name} · ${role}` : undefined}
        className={cn(
          'flex cursor-pointer items-center rounded-[7px] text-left outline-none',
          'transition-colors duration-150 ease-out hover:bg-[var(--ref-hover)]',
          'focus-visible:ring-2 focus-visible:ring-white/25',
          collapsed ? 'size-9 justify-center' : 'w-full gap-2.5 p-1',
        )}
      >
        <WorkspaceAvatar size={collapsed ? 24 : 30} />

        {collapsed ? null : (
          <>
            <span className="min-w-0 flex-1">
              <span className="block truncate text-[13.5px] font-semibold leading-[18px] text-[var(--ref-text)]">
                {name}
              </span>
              <span className="block truncate text-[12.5px] leading-[17px] text-[var(--ref-text-dim)]">
                {role}
              </span>
            </span>
            <ChevronsUpDown
              aria-hidden="true"
              className="size-[14px] shrink-0 text-[var(--ref-icon)]"
            />
          </>
        )}
      </button>
    </div>
  );
}
