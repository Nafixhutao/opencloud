'use client';

import type { SidebarNavItem } from '@/planing-ui/sidebar/nav-data';
import { SidebarItem } from '@/planing-ui/sidebar/sidebar-item';
import { SidebarSection } from '@/planing-ui/sidebar/sidebar-section';
import { cn } from '@/lib/utils';

type SidebarMenuProps = {
  items: SidebarNavItem[];
  collapsed: boolean;
  className?: string;
};

/**
 * Renders a flat list of rows, delegating any collapsible entry to
 * `SidebarSection` so it can own its expanded state.
 */
export function SidebarMenu({ items, collapsed, className }: SidebarMenuProps) {
  return (
    <ul className={cn('m-0 flex list-none flex-col gap-[2px] p-0', className)}>
      {items.map((item) =>
        item.collapsible ? (
          <SidebarSection key={item.label} item={item} collapsed={collapsed} />
        ) : (
          <li key={item.label}>
            <SidebarItem
              label={item.label}
              icon={item.icon}
              badge={item.badge}
              active={item.active}
              selected={item.selected}
              collapsed={collapsed}
            />
          </li>
        ),
      )}
    </ul>
  );
}
