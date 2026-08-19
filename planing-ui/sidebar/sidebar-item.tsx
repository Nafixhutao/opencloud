'use client';

import { ChevronDown } from 'lucide-react';

import type { SidebarBadge, SidebarIcon } from '@/planing-ui/sidebar/nav-data';
import { cn } from '@/lib/utils';

type SidebarItemProps = {
  label: string;
  icon?: SidebarIcon;
  badge?: SidebarBadge;
  /** Lets a section point its submenu's `aria-labelledby` at this row. */
  id?: string;
  /** Renders the trailing chevron and wires up accordion semantics. */
  collapsible?: boolean;
  expanded?: boolean;
  /** Accent-colored label, as "Integrations" in the reference. */
  active?: boolean;
  /** Boxed row with a lighter fill, as "Repositories" in the reference. */
  selected?: boolean;
  /** Submenu rows sit at a smaller size with no icon slot. */
  variant?: 'main' | 'sub';
  /** Icon-rail mode: centers the icon and hides label, badge, and chevron. */
  collapsed?: boolean;
  onToggle?: () => void;
};

/**
 * One clickable navigation row: icon, label, optional badge, optional chevron.
 *
 * Rows are always `<button>` — this sidebar is a visual reference with no real
 * destinations, so nothing navigates. Colors come from the `--ref-*` variables
 * scoped on the sidebar root rather than global theme tokens.
 */
export function SidebarItem({
  label,
  icon: Icon,
  badge,
  id,
  collapsible = false,
  expanded = false,
  active = false,
  selected = false,
  variant = 'main',
  collapsed = false,
  onToggle,
}: SidebarItemProps) {
  const isSub = variant === 'sub';

  return (
    <button
      type="button"
      id={id}
      onClick={onToggle}
      aria-expanded={collapsible && !collapsed ? expanded : undefined}
      aria-current={active || selected ? 'page' : undefined}
      // At rail width the label is visually hidden, so the icon needs a name.
      aria-label={collapsed ? label : undefined}
      title={collapsed ? label : undefined}
      className={cn(
        'group relative flex w-full cursor-pointer items-center rounded-[7px] text-left outline-none',
        'transition-colors duration-150 ease-out',
        'focus-visible:ring-2 focus-visible:ring-white/25',
        isSub ? 'h-[34px] text-[14px]' : 'h-[34px] text-[14px]',
        collapsed ? 'justify-center px-0' : isSub ? 'gap-2 px-2' : 'gap-2.5 px-2',
        selected
          ? 'bg-[var(--ref-selected-bg)] font-medium text-[var(--ref-text)]'
          : active
            ? 'font-normal text-[var(--ref-active)] hover:bg-[var(--ref-hover)]'
            : 'font-normal text-[var(--ref-text)] hover:bg-[var(--ref-hover)]',
      )}
    >
      {Icon ? (
        <span
          aria-hidden="true"
          className={cn(
            'grid size-[18px] shrink-0 place-items-center',
            'transition-colors duration-150 ease-out',
            active
              ? 'text-[var(--ref-active)]'
              : selected
                ? 'text-[var(--ref-text)]'
                : 'text-[var(--ref-icon)] group-hover:text-[var(--ref-text)]',
          )}
        >
          <Icon className="size-[16px]" />
        </span>
      ) : null}

      {/* Kept mounted and faded out so the rail transition stays smooth. */}
      <span
        className={cn(
          'min-w-0 flex-1 truncate transition-[opacity,transform] duration-150 ease-out',
          collapsed && 'pointer-events-none absolute -translate-x-1 opacity-0',
        )}
      >
        {label}
      </span>

      {badge && !collapsed ? <SidebarItemBadge badge={badge} /> : null}

      {collapsible && !collapsed ? (
        <ChevronDown
          aria-hidden="true"
          className={cn(
            'size-[14px] shrink-0 text-[var(--ref-icon)]',
            'transition-transform duration-200 ease-out',
            expanded && 'rotate-180',
          )}
        />
      ) : null}
    </button>
  );
}

/** Small pill beside a label: gray for "Beta", dark orange for "New". */
function SidebarItemBadge({ badge }: { badge: SidebarBadge }) {
  return (
    <span
      className={cn(
        'shrink-0 rounded-[5px] px-1.5 py-[1px] text-[11px] font-semibold leading-[16px]',
        badge.tone === 'new'
          ? 'bg-[var(--ref-new-bg)] text-[var(--ref-new-text)]'
          : 'bg-[var(--ref-beta-bg)] text-[var(--ref-beta-text)]',
      )}
    >
      {badge.label}
    </span>
  );
}
