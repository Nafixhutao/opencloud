'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { useEffect, useState } from 'react';

import {
  AnimatedSidebarMenu,
  AnimatedSidebarMenuButton,
  AnimatedSidebarMenuItem,
  AnimatedSidebarMenuSub,
  AnimatedSidebarMenuSubButton,
  AnimatedSidebarMenuSubItem,
  useAnimatedSidebarPanel,
} from '@/components/motion/animated-sidebar';
import { Badge } from '@/components/ui/badge';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuLinkItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { isHrefActive, isItemActive, type NavItem } from '@/lib/navigation';

type NavMainProps = {
  items: NavItem[];
};

export function NavMain({ items }: NavMainProps) {
  return (
    <AnimatedSidebarMenu>
      {items.map((item) => (
        <NavRow key={item.label} item={item} />
      ))}
    </AnimatedSidebarMenu>
  );
}

function NavRow({ item }: { item: NavItem }) {
  const pathname = usePathname();
  const { collapsed } = useAnimatedSidebarPanel();
  const Icon = item.icon;

  const hasChildren = (item.items?.length ?? 0) > 0;
  const active = isItemActive(pathname, item);
  const badge = item.badge ? (
    <Badge variant={item.badge.variant ?? 'secondary'}>{item.badge.label}</Badge>
  ) : undefined;

  // Parents open when they own the current route, and reopen if navigation
  // moves into their subtree from elsewhere.
  const [open, setOpen] = useState(active);
  useEffect(() => {
    if (active) setOpen(true);
  }, [active]);

  if (!hasChildren) {
    return (
      <AnimatedSidebarMenuItem>
        <AnimatedSidebarMenuButton
          href={item.href}
          linkComponent={item.href ? Link : undefined}
          icon={<Icon className="size-4" />}
          badge={badge}
          isActive={active}
        >
          {item.label}
        </AnimatedSidebarMenuButton>
      </AnimatedSidebarMenuItem>
    );
  }

  // The inline submenu is hidden at icon width, so a collapsed parent opens its
  // children in a flyout instead of becoming unreachable.
  if (collapsed) {
    return (
      <AnimatedSidebarMenuItem>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <AnimatedSidebarMenuButton
                icon={<Icon className="size-4" />}
                isActive={active}
              >
                {item.label}
              </AnimatedSidebarMenuButton>
            }
          />
          <DropdownMenuContent side="right" align="start" sideOffset={8}>
            <DropdownMenuLabel>{item.label}</DropdownMenuLabel>
            <DropdownMenuSeparator />
            {(item.items ?? []).map((child) => (
              <DropdownMenuLinkItem
                key={child.label}
                render={<Link href={child.href ?? '#'} />}
                aria-current={
                  child.href && isHrefActive(pathname, child.href, child.exact)
                    ? 'page'
                    : undefined
                }
              >
                <child.icon />
                {child.label}
              </DropdownMenuLinkItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      </AnimatedSidebarMenuItem>
    );
  }

  return (
    <AnimatedSidebarMenuItem>
      <AnimatedSidebarMenuButton
        icon={<Icon className="size-4" />}
        badge={badge}
        isActive={active}
        ariaExpanded={open}
        onSelect={() => setOpen((value) => !value)}
      >
        {item.label}
      </AnimatedSidebarMenuButton>
      <AnimatedSidebarMenuSub open={open}>
        {(item.items ?? []).map((child) => (
          <AnimatedSidebarMenuSubItem key={child.label}>
            <AnimatedSidebarMenuSubButton
              href={child.href}
              linkComponent={child.href ? Link : undefined}
              icon={<child.icon className="size-3.5" />}
              isActive={
                Boolean(child.href) &&
                isHrefActive(pathname, child.href as string, child.exact)
              }
            >
              {child.label}
            </AnimatedSidebarMenuSubButton>
          </AnimatedSidebarMenuSubItem>
        ))}
      </AnimatedSidebarMenuSub>
    </AnimatedSidebarMenuItem>
  );
}
