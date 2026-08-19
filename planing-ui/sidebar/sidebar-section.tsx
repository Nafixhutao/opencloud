'use client';

import { useId, useState } from 'react';

import type { SidebarNavItem } from '@/planing-ui/sidebar/nav-data';
import { SidebarItem } from '@/planing-ui/sidebar/sidebar-item';
import { SidebarSubmenu } from '@/planing-ui/sidebar/sidebar-submenu';

type SidebarSectionProps = {
  item: SidebarNavItem;
  collapsed: boolean;
};

/**
 * A collapsible row plus its child list — "Review" in the reference, which is
 * expanded on first paint. Each section owns its own open state so sections
 * toggle independently (the reference is not a single-open accordion).
 */
export function SidebarSection({ item, collapsed }: SidebarSectionProps) {
  const [open, setOpen] = useState(item.defaultOpen ?? false);
  const triggerId = useId();
  const hasItems = (item.items?.length ?? 0) > 0;

  return (
    <li className="relative">
      <SidebarItem
        id={triggerId}
        label={item.label}
        icon={item.icon}
        badge={item.badge}
        collapsible
        expanded={open}
        collapsed={collapsed}
        onToggle={() => setOpen((value) => !value)}
      />

      {/* The rail has no room for children; the row alone remains. */}
      {hasItems && !collapsed ? (
        <SidebarSubmenu
          items={item.items ?? []}
          open={open}
          labelledBy={triggerId}
        />
      ) : null}
    </li>
  );
}
