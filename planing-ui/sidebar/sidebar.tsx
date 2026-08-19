'use client';

import { ChevronDown, Menu } from 'lucide-react';
import { useState } from 'react';

import {
  overflowNav,
  primaryNav,
  secondaryNav,
} from '@/planing-ui/sidebar/nav-data';
import { SidebarFooter } from '@/planing-ui/sidebar/sidebar-footer';
import { SidebarHeader } from '@/planing-ui/sidebar/sidebar-header';
import { SidebarMenu } from '@/planing-ui/sidebar/sidebar-menu';
import { cn } from '@/lib/utils';

import './sidebar.css';

/**
 * Palette for the reference sidebar, scoped to this component only.
 *
 * `app/globals.css` defines a deliberately monochrome theme ("color survives
 * only as status meaning"). The reference design needs a near-black surface and
 * an orange "New" badge, so those values live here as local custom properties
 * instead of global tokens — nothing outside this subtree is affected.
 */
const REF_THEME = {
  '--ref-bg': '#0d0b10',
  '--ref-text': '#e8e8ea',
  '--ref-text-dim': '#8a8a94',
  '--ref-icon': '#9a9aa4',
  '--ref-active': '#8ab4f8',
  '--ref-hover': 'rgb(255 255 255 / 0.045)',
  '--ref-selected-bg': 'rgb(255 255 255 / 0.075)',
  '--ref-divider': 'rgb(255 255 255 / 0.07)',
  '--ref-beta-bg': '#26262c',
  '--ref-beta-text': '#c4c4cc',
  '--ref-plan-text': '#b8b8c0',
  '--ref-new-bg': '#7c3a12',
  '--ref-new-text': '#f7a95c',
  '--ref-menu-bg': '#17151b',
  '--ref-menu-border': 'rgb(255 255 255 / 0.09)',
  '--ref-menu-active': 'rgb(255 255 255 / 0.07)',
  '--ref-scroll-thumb': 'rgb(255 255 255 / 0.13)',
} as React.CSSProperties;

const EXPANDED_WIDTH = 268;
const RAIL_WIDTH = 52;

type SidebarProps = {
  name?: string;
  plan?: string;
  userName?: string;
  userRole?: string;
  /** Start collapsed to the icon rail. */
  defaultCollapsed?: boolean;
};

/**
 * Reference sidebar — a visual replica of the supplied design screenshots.
 *
 * Desktop: fixed panel that animates between a full panel and a 52px icon rail.
 * Below `md` it becomes an off-canvas drawer with a backdrop and a hamburger
 * trigger. Rows do not navigate; see `nav-data.ts` for why.
 */
export function Sidebar({
  name = 'nazxf',
  plan = 'Pro Plus',
  userName = 'Nazxf',
  userRole = 'Admin',
  defaultCollapsed = false,
}: SidebarProps) {
  const [collapsed, setCollapsed] = useState(defaultCollapsed);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [showOverflow, setShowOverflow] = useState(false);

  return (
    <>
      {/* Mobile trigger — hidden once the panel is permanently visible. */}
      <button
        type="button"
        onClick={() => setDrawerOpen(true)}
        aria-label="Open navigation"
        aria-expanded={drawerOpen}
        className={cn(
          'fixed top-3 left-3 z-50 grid size-9 cursor-pointer place-items-center rounded-[7px] md:hidden',
          'border border-[rgb(255_255_255/0.08)] bg-[#0d0b10] text-[#9a9aa4] outline-none',
          'transition-colors duration-150 ease-out hover:text-[#e8e8ea]',
          'focus-visible:ring-2 focus-visible:ring-white/25',
        )}
      >
        <Menu aria-hidden="true" className="size-[18px]" />
      </button>

      {/* Backdrop, drawer only. */}
      <button
        type="button"
        tabIndex={drawerOpen ? 0 : -1}
        aria-label="Close navigation"
        onClick={() => setDrawerOpen(false)}
        className={cn(
          'fixed inset-0 z-40 bg-black/60 transition-opacity duration-200 ease-out md:hidden',
          drawerOpen ? 'opacity-100' : 'pointer-events-none opacity-0',
        )}
      />

      <nav
        aria-label="Reference navigation"
        data-collapsed={collapsed}
        style={{
          ...REF_THEME,
          // Animating width is what produces the rail transition in the
          // reference; the drawer is always full width on mobile.
          width: collapsed ? RAIL_WIDTH : EXPANDED_WIDTH,
        }}
        className={cn(
          'ref-sidebar fixed inset-y-0 left-0 z-40 flex h-screen shrink-0 flex-col',
          'border-r border-[var(--ref-divider)] bg-[var(--ref-bg)]',
          'font-sans text-[var(--ref-text)] antialiased',
          'transition-[width,transform] duration-200 ease-out will-change-[width,transform]',
          drawerOpen ? 'translate-x-0' : '-translate-x-full',
          // From md up the panel is always in view and part of the layout flow.
          'md:relative md:translate-x-0',
        )}
      >
        <SidebarHeader
          name={name}
          plan={plan}
          collapsed={collapsed}
          onTogglePanel={() => setCollapsed((value) => !value)}
        />

        {/* Scroll region: everything between header and footer. */}
        <div
          className={cn(
            'ref-scroll flex min-h-0 flex-1 flex-col overflow-y-auto overflow-x-hidden pb-1',
            collapsed ? 'px-[7px]' : 'px-[10px]',
          )}
        >
          <SidebarMenu items={primaryNav} collapsed={collapsed} />

          <Divider className="my-[7px]" />

          <SidebarMenu items={secondaryNav} collapsed={collapsed} />

          {showOverflow ? (
            <SidebarMenu
              items={overflowNav}
              collapsed={collapsed}
              className="mt-[2px]"
            />
          ) : null}

          <ViewMore
            expanded={showOverflow}
            collapsed={collapsed}
            onToggle={() => setShowOverflow((value) => !value)}
          />
        </div>

        <SidebarFooter name={userName} role={userRole} collapsed={collapsed} />
      </nav>
    </>
  );
}

function Divider({ className }: { className?: string }) {
  return (
    <div
      aria-hidden="true"
      className={cn('h-px shrink-0 bg-[var(--ref-divider)]', className)}
    />
  );
}

/**
 * Divider with the rounded pill centered on top of it, matching the reference
 * where the pill visually interrupts the line. At rail width the label would
 * not fit, so only a chevron button remains.
 */
function ViewMore({
  expanded,
  collapsed,
  onToggle,
}: {
  expanded: boolean;
  collapsed: boolean;
  onToggle: () => void;
}) {
  return (
    <div className="relative mt-[9px] mb-[3px] flex shrink-0 items-center justify-center">
      <div
        aria-hidden="true"
        className="absolute inset-x-0 top-1/2 h-px -translate-y-1/2 bg-[var(--ref-divider)]"
      />
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={expanded}
        aria-label={collapsed ? (expanded ? 'View less' : 'View more') : undefined}
        className={cn(
          'relative z-10 inline-flex cursor-pointer items-center gap-1 rounded-full outline-none',
          'border border-[var(--ref-divider)] bg-[#16141a]',
          'text-[12px] font-medium text-[var(--ref-text-dim)]',
          'transition-colors duration-150 ease-out hover:text-[var(--ref-text)]',
          'focus-visible:ring-2 focus-visible:ring-white/25',
          collapsed ? 'size-6 justify-center' : 'px-2.5 py-[3px]',
        )}
      >
        <ChevronDown
          aria-hidden="true"
          className={cn(
            'size-[13px] shrink-0 transition-transform duration-200 ease-out',
            expanded && 'rotate-180',
          )}
        />
        {collapsed ? null : expanded ? 'View less' : 'View more'}
      </button>
    </div>
  );
}
