import type { LucideIcon } from 'lucide-react';
import {
  BookOpen,
  ChartPie,
  ClipboardList,
  FileText,
  Headphones,
  House,
  Search,
  Shield,
  Users,
} from 'lucide-react';
import type { ComponentType, SVGProps } from 'react';

import { DiscordIcon, SlackIcon } from '@/planing-ui/sidebar/brand-icons';

/**
 * Menu tree for the reference sidebar, transcribed from the design screenshot.
 *
 * This is a visual reference only, so no item carries an `href`: none of these
 * labels maps to a real OpenCloud route, and UI_GUIDELINES §3.1 treats a nav
 * entry that 404s as a bug. Rows render as inert buttons instead. Production
 * navigation lives in `lib/navigation.ts` and is untouched by this file.
 */

/** Lucide icons and the inline brand marks share this shape. */
export type SidebarIcon = LucideIcon | ComponentType<SVGProps<SVGSVGElement>>;

export type SidebarBadge = {
  label: string;
  /** `beta` is dark gray, `new` is dark orange — matching the reference. */
  tone: 'beta' | 'new';
};

export type SidebarNavItem = {
  label: string;
  icon?: SidebarIcon;
  badge?: SidebarBadge;
  /** Renders a chevron and makes the row an accordion trigger. */
  collapsible?: boolean;
  /** Open on first paint. Only Review starts expanded in the reference. */
  defaultOpen?: boolean;
  /** Lit in accent blue, as "Integrations" is in the screenshot. */
  active?: boolean;
  /** Boxed lighter fill, as "Repositories" is in the screenshot. */
  selected?: boolean;
  items?: SidebarNavItem[];
};

/** Rows above the first divider. */
export const primaryNav: SidebarNavItem[] = [
  { label: 'Search', icon: Search },
  { label: 'Explore', icon: House },
  { label: 'Analytics', icon: ChartPie, collapsible: true },
];

/** Rows below the first divider, ending at the "View more" pill. */
export const secondaryNav: SidebarNavItem[] = [
  {
    label: 'Review',
    icon: FileText,
    collapsible: true,
    defaultOpen: true,
    // Submenu rows are label-only in the reference — no leading icons.
    items: [
      { label: 'Triage', badge: { label: 'Beta', tone: 'beta' } },
      { label: 'Repositories', selected: true },
      { label: 'Integrations', active: true },
      { label: 'Learnings' },
      { label: 'Caches' },
      { label: 'Organization Settings' },
    ],
  },
  {
    label: 'Slack',
    icon: SlackIcon,
    badge: { label: 'New', tone: 'new' },
    collapsible: true,
  },
  { label: 'Discord', icon: DiscordIcon, collapsible: true },
  {
    label: 'Security',
    icon: Shield,
    badge: { label: 'New', tone: 'new' },
    collapsible: true,
  },
  { label: 'Plan', icon: ClipboardList, collapsible: true },
];

/**
 * Revealed by the "View more" pill. These are the extra rail icons visible in
 * the collapsed reference (people, docs, support).
 */
export const overflowNav: SidebarNavItem[] = [
  { label: 'Members', icon: Users },
  { label: 'Docs', icon: BookOpen },
  { label: 'Support', icon: Headphones },
];
