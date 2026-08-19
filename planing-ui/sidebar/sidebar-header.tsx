'use client';

import { Check, ChevronsUpDown, PanelLeft, Plus, RefreshCw } from 'lucide-react';

import { WorkspaceAvatar } from '@/planing-ui/sidebar/workspace-avatar';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { cn } from '@/lib/utils';

type SidebarHeaderProps = {
  name: string;
  plan: string;
  collapsed: boolean;
  onTogglePanel: () => void;
};

/**
 * Workspace row: avatar, name, plan badge, an organization switcher, and the
 * panel toggle. At rail width only the avatar and toggle remain.
 */
export function SidebarHeader({
  name,
  plan,
  collapsed,
  onTogglePanel,
}: SidebarHeaderProps) {
  // The rail shows just the toggle, matching the collapsed reference.
  if (collapsed) {
    return (
      <div className="flex h-[52px] shrink-0 items-center justify-center">
        <PanelToggle onClick={onTogglePanel} label="Expand sidebar" />
      </div>
    );
  }

  return (
    <div className="flex h-[52px] shrink-0 items-center gap-1 px-[12px]">
      <DropdownMenu>
        <DropdownMenuTrigger
          className={cn(
            'flex min-w-0 flex-1 cursor-pointer items-center gap-2 rounded-[7px] py-1 pr-1 pl-1 text-left outline-none',
            'transition-colors duration-150 ease-out hover:bg-[var(--ref-hover)]',
            'focus-visible:ring-2 focus-visible:ring-white/25',
          )}
        >
          <WorkspaceAvatar size={22} />
          <span className="truncate text-[14px] font-semibold text-[var(--ref-text)]">
            {name}
          </span>
          <span className="shrink-0 rounded-[5px] bg-[var(--ref-beta-bg)] px-1.5 py-[1px] text-[11px] font-medium leading-[16px] text-[var(--ref-plan-text)]">
            {plan}
          </span>
          <ChevronsUpDown
            aria-hidden="true"
            className="size-[13px] shrink-0 text-[var(--ref-icon)]"
          />
        </DropdownMenuTrigger>

        {/* Width tracks the trigger so the panel spans the sidebar, as in the
            reference where it visually overlays the menu below. */}
        <DropdownMenuContent
          side="bottom"
          align="start"
          sideOffset={4}
          className="w-[var(--anchor-width)] min-w-[236px] rounded-[10px] border border-[var(--ref-menu-border)] bg-[var(--ref-menu-bg)] p-1.5 ring-0"
        >
          <DropdownMenuItem
            className={cn(
              'flex h-9 cursor-pointer items-center gap-2 rounded-[7px] px-2.5 text-[14px]',
              'bg-[var(--ref-menu-active)] font-medium text-[var(--ref-text)]',
              'data-highlighted:bg-[var(--ref-menu-active)]',
            )}
          >
            <span className="min-w-0 flex-1 truncate">{name}</span>
            <Check aria-hidden="true" className="size-[15px] shrink-0" />
          </DropdownMenuItem>

          <DropdownMenuSeparator className="my-1.5 bg-[var(--ref-menu-border)]" />

          <DropdownMenuItem
            className={cn(
              'flex h-9 cursor-pointer items-center gap-2.5 rounded-[7px] px-2.5 text-[14px]',
              'text-[var(--ref-text)] data-highlighted:bg-[var(--ref-hover)]',
            )}
          >
            <RefreshCw
              aria-hidden="true"
              className="size-[15px] shrink-0 text-[var(--ref-icon)]"
            />
            Refresh list
          </DropdownMenuItem>

          <DropdownMenuItem
            className={cn(
              'flex h-9 cursor-pointer items-center gap-2.5 rounded-[7px] px-2.5 text-[14px]',
              'text-[var(--ref-text)] data-highlighted:bg-[var(--ref-hover)]',
            )}
          >
            <Plus
              aria-hidden="true"
              className="size-[15px] shrink-0 text-[var(--ref-icon)]"
            />
            Add organization
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <PanelToggle onClick={onTogglePanel} label="Collapse sidebar" />
    </div>
  );
}

function PanelToggle({
  onClick,
  label,
}: {
  onClick: () => void;
  label: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={label}
      className={cn(
        'grid size-7 shrink-0 cursor-pointer place-items-center rounded-[7px] outline-none',
        'text-[var(--ref-icon)] transition-colors duration-150 ease-out',
        'hover:bg-[var(--ref-hover)] hover:text-[var(--ref-text)]',
        'focus-visible:ring-2 focus-visible:ring-white/25',
      )}
    >
      <PanelLeft aria-hidden="true" className="size-[16px]" />
    </button>
  );
}
