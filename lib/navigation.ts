import type { LucideIcon } from 'lucide-react';
import {
  Database,
  FolderKanban,
  Globe,
  LayoutDashboard,
  ShieldCheck,
  UserCog,
  Users,
} from 'lucide-react';

/**
 * Single source of truth for dashboard sidebar navigation.
 *
 * Only routes that actually exist are listed here. Product areas that have an
 * API but no page (for example domains, which is embedded in a site detail)
 * stay out until they have a real destination — a nav entry that 404s is worse
 * than no nav entry.
 */

export type NavBadge = {
  label: string;
  variant?: 'default' | 'secondary' | 'outline';
};

export type NavItem = {
  label: string;
  icon: LucideIcon;
  /** Omitted for parents that only group children. */
  href?: string;
  badge?: NavBadge;
  items?: NavItem[];
  /** Match `pathname === href` instead of the prefix rule. */
  exact?: boolean;
};

export type NavGroup = {
  id: string;
  label?: string;
  items: NavItem[];
  /** Collapsed behind the "View more" control until the user opts in. */
  secondary?: boolean;
};

export type BuildNavOptions = {
  isAdmin: boolean;
};

export function buildNav({ isAdmin }: BuildNavOptions): NavGroup[] {
  const groups: NavGroup[] = [
    {
      id: 'platform',
      items: [
        { label: 'Overview', href: '/dashboard', icon: LayoutDashboard, exact: true },
        { label: 'Projects', href: '/projects', icon: FolderKanban },
        { label: 'Databases', href: '/databases', icon: Database },
        { label: 'Sites', href: '/sites', icon: Globe },
      ],
    },
  ];

  const workspaceItems: NavItem[] = [
    { label: 'Account', href: '/account', icon: UserCog },
  ];

  if (isAdmin) {
    workspaceItems.push({
      label: 'Administration',
      icon: ShieldCheck,
      badge: { label: 'Admin', variant: 'outline' },
      items: [{ label: 'Users', href: '/admin/users', icon: Users }],
    });
  }

  groups.push({
    id: 'workspace',
    label: 'Workspace',
    secondary: true,
    items: workspaceItems,
  });

  return groups;
}

/**
 * True when `pathname` points at this item's own route.
 *
 * Exact items match only themselves so that `/dashboard` does not stay lit on
 * every nested route. Everything else matches its subtree, but on a segment
 * boundary — `/sites` must not light up for a hypothetical `/sites-archive`.
 */
export function isHrefActive(
  pathname: string,
  href: string,
  exact = false,
): boolean {
  if (pathname === href) return true;
  if (exact) return false;
  return pathname.startsWith(`${href}/`);
}

/**
 * True when the item or any of its descendants is active, so a collapsed parent
 * still reads as current and can auto-open on load.
 */
export function isItemActive(pathname: string, item: NavItem): boolean {
  if (item.href && isHrefActive(pathname, item.href, item.exact)) {
    return true;
  }
  return (item.items ?? []).some((child) => isItemActive(pathname, child));
}

/** Ids of every group holding an active item — used to seed open state. */
export function activeGroupIds(pathname: string, groups: NavGroup[]): string[] {
  return groups
    .filter((group) => group.items.some((item) => isItemActive(pathname, item)))
    .map((group) => group.id);
}

/** Case-insensitive label filter that keeps a parent when a child matches. */
export function filterNavItems(items: NavItem[], query: string): NavItem[] {
  const needle = query.trim().toLowerCase();
  if (!needle) return items;

  return items.reduce<NavItem[]>((matches, item) => {
    const children = filterNavItems(item.items ?? [], query);
    if (item.label.toLowerCase().includes(needle)) {
      matches.push(item);
    } else if (children.length > 0) {
      matches.push({ ...item, items: children });
    }
    return matches;
  }, []);
}
