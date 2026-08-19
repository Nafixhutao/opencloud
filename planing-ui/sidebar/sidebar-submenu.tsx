'use client';

import type { SidebarNavItem } from '@/planing-ui/sidebar/nav-data';
import { SidebarItem } from '@/planing-ui/sidebar/sidebar-item';

type SidebarSubmenuProps = {
  items: SidebarNavItem[];
  /** Drives the collapse animation; the list stays mounted when closed. */
  open: boolean;
  labelledBy?: string;
};

/**
 * Indented child list with the thin vertical indicator line on its left edge,
 * as in the reference. Height is animated via a grid-rows transition so the
 * open/close motion stays smooth without measuring content.
 */
export function SidebarSubmenu({ items, open, labelledBy }: SidebarSubmenuProps) {
  return (
    <div
      aria-labelledby={labelledBy}
      className="grid transition-[grid-template-rows] duration-200 ease-out"
      style={{ gridTemplateRows: open ? '1fr' : '0fr' }}
    >
      <ul
        // `min-h-0` lets the 0fr row actually collapse; overflow hides the
        // children mid-transition. `inert` keeps collapsed rows out of the tab
        // order and the a11y tree while they stay mounted for the animation.
        className="m-0 mt-0.5 ml-[18px] min-h-0 list-none overflow-hidden border-l border-[var(--ref-divider)] pl-3"
        inert={!open}
      >
        {items.map((item) => (
          <li key={item.label} className="relative">
            <SidebarItem
              label={item.label}
              icon={item.icon}
              badge={item.badge}
              active={item.active}
              selected={item.selected}
              variant="sub"
            />
          </li>
        ))}
      </ul>
    </div>
  );
}
