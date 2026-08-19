import { Database, LayoutDashboard } from 'lucide-react';
import { describe, expect, it } from 'vitest';

import {
  activeGroupIds,
  buildNav,
  filterNavItems,
  isHrefActive,
  isItemActive,
  type NavItem,
} from '@/lib/navigation';

describe('isHrefActive', () => {
  it('matches an exact item only on its own route', () => {
    expect(isHrefActive('/dashboard', '/dashboard', true)).toBe(true);
    expect(isHrefActive('/dashboard/anything', '/dashboard', true)).toBe(false);
  });

  it('matches a prefix item on its own route and its subtree', () => {
    expect(isHrefActive('/projects', '/projects')).toBe(true);
    expect(isHrefActive('/projects/abc', '/projects')).toBe(true);
    expect(isHrefActive('/projects/abc/storage', '/projects')).toBe(true);
  });

  it('only matches on a segment boundary', () => {
    expect(isHrefActive('/sites-archive', '/sites')).toBe(false);
    expect(isHrefActive('/projectsomething', '/projects')).toBe(false);
  });

  it('does not match an unrelated route', () => {
    expect(isHrefActive('/databases', '/sites')).toBe(false);
  });
});

describe('isItemActive', () => {
  const parent: NavItem = {
    label: 'Administration',
    icon: Database,
    items: [{ label: 'Users', href: '/admin/users', icon: Database }],
  };

  it('is true for a leaf on its own route', () => {
    const leaf: NavItem = { label: 'Databases', href: '/databases', icon: Database };
    expect(isItemActive('/databases', leaf)).toBe(true);
  });

  it('is true for a hrefless parent when a child matches', () => {
    expect(isItemActive('/admin/users', parent)).toBe(true);
  });

  it('is false for a hrefless parent when no child matches', () => {
    expect(isItemActive('/dashboard', parent)).toBe(false);
  });

  it('respects the exact flag through the tree', () => {
    const item: NavItem = {
      label: 'Overview',
      href: '/dashboard',
      icon: LayoutDashboard,
      exact: true,
    };
    expect(isItemActive('/dashboard', item)).toBe(true);
    expect(isItemActive('/dashboard/x', item)).toBe(false);
  });
});

describe('buildNav', () => {
  it('omits administration for non-admins', () => {
    const groups = buildNav({ isAdmin: false });
    const labels = groups.flatMap((group) => group.items.map((item) => item.label));
    expect(labels).not.toContain('Administration');
    expect(labels).toContain('Account');
  });

  it('includes a badged administration group for admins', () => {
    const groups = buildNav({ isAdmin: true });
    const admin = groups
      .flatMap((group) => group.items)
      .find((item) => item.label === 'Administration');
    expect(admin?.badge?.label).toBe('Admin');
    expect(admin?.items?.[0]?.href).toBe('/admin/users');
  });

  it('marks the workspace group as secondary so it hides behind View more', () => {
    const groups = buildNav({ isAdmin: true });
    expect(groups.find((group) => group.id === 'platform')?.secondary).toBeUndefined();
    expect(groups.find((group) => group.id === 'workspace')?.secondary).toBe(true);
  });

  it('only points at routes that exist', () => {
    const hrefs = buildNav({ isAdmin: true })
      .flatMap((group) => group.items)
      .flatMap((item) => [item.href, ...(item.items ?? []).map((child) => child.href)])
      .filter((href): href is string => Boolean(href));

    expect(hrefs).toEqual([
      '/dashboard',
      '/projects',
      '/databases',
      '/sites',
      '/account',
      '/admin/users',
    ]);
  });
});

describe('activeGroupIds', () => {
  it('reports the group owning the active route', () => {
    const groups = buildNav({ isAdmin: true });
    expect(activeGroupIds('/databases', groups)).toEqual(['platform']);
    expect(activeGroupIds('/admin/users', groups)).toEqual(['workspace']);
  });

  it('reports nothing for a route outside the nav', () => {
    expect(activeGroupIds('/login', buildNav({ isAdmin: true }))).toEqual([]);
  });
});

describe('filterNavItems', () => {
  const items: NavItem[] = [
    { label: 'Databases', href: '/databases', icon: Database },
    {
      label: 'Administration',
      icon: Database,
      items: [{ label: 'Users', href: '/admin/users', icon: Database }],
    },
  ];

  it('returns everything for an empty query', () => {
    expect(filterNavItems(items, '   ')).toHaveLength(2);
  });

  it('matches labels case-insensitively', () => {
    expect(filterNavItems(items, 'DATA').map((item) => item.label)).toEqual([
      'Databases',
    ]);
  });

  it('keeps a parent and narrows it to the matching child', () => {
    const result = filterNavItems(items, 'users');
    expect(result).toHaveLength(1);
    expect(result[0]?.label).toBe('Administration');
    expect(result[0]?.items?.map((child) => child.label)).toEqual(['Users']);
  });

  it('returns nothing when no label matches', () => {
    expect(filterNavItems(items, 'zzz')).toEqual([]);
  });
});
