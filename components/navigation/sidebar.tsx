'use client';

import { Database, FolderKanban, Globe, LayoutDashboard, UserCog, Users } from 'lucide-react';
import { usePathname } from 'next/navigation';

import { SignOutButton } from '@/components/auth/sign-out-button';
import {
  AnimatedSidebar,
  AnimatedSidebarContent,
  AnimatedSidebarFooter,
  AnimatedSidebarGroup,
  AnimatedSidebarGroupContent,
  AnimatedSidebarHeader,
  AnimatedSidebarMenu,
  AnimatedSidebarMenuButton,
  AnimatedSidebarMenuItem,
} from '@/components/motion/animated-sidebar';

type SidebarProps = {
  email: string;
  isAdmin: boolean;
};

// App navigation on the beUI animated-sidebar primitives: icon-rail collapse,
// spring active pill, built-in mobile sheet, and Ctrl/Cmd+B toggle.
export function Sidebar({ email, isAdmin }: SidebarProps) {
  const pathname = usePathname();

  const links = [
    { href: '/dashboard', label: 'Overview', icon: LayoutDashboard },
    { href: '/projects', label: 'Projects', icon: FolderKanban },
    { href: '/databases', label: 'Databases', icon: Database },
    { href: '/sites', label: 'Sites', icon: Globe },
    { href: '/account', label: 'Account', icon: UserCog },
    ...(isAdmin ? [{ href: '/admin/users', label: 'Users', icon: Users }] : []),
  ];

  const isActive = (href: string) => {
    if (href === '/dashboard') return pathname === '/dashboard';
    return pathname.startsWith(href);
  };

  return (
    <AnimatedSidebar ariaLabel="OpenCloud navigation" collapsible="icon">
      <AnimatedSidebarHeader>
        <div className="flex h-10 items-center gap-2 px-2">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-md bg-primary text-sm font-medium tracking-tight text-primary-foreground">
            OC
          </div>
          <span className="truncate text-sm font-medium tracking-tight text-sidebar-foreground">
            OpenCloud
          </span>
        </div>
      </AnimatedSidebarHeader>

      <AnimatedSidebarContent>
        <AnimatedSidebarGroup>
          <AnimatedSidebarGroupContent>
            <AnimatedSidebarMenu>
              {links.map(({ href, label, icon: Icon }) => (
                <AnimatedSidebarMenuItem key={href}>
                  <AnimatedSidebarMenuButton
                    href={href}
                    icon={<Icon className="size-4" />}
                    isActive={isActive(href)}
                  >
                    {label}
                  </AnimatedSidebarMenuButton>
                </AnimatedSidebarMenuItem>
              ))}
            </AnimatedSidebarMenu>
          </AnimatedSidebarGroupContent>
        </AnimatedSidebarGroup>
      </AnimatedSidebarContent>

      <AnimatedSidebarFooter>
        <div className="flex items-center gap-2 px-2 pb-2">
          <div className="flex size-7 shrink-0 items-center justify-center rounded-full bg-muted text-xs text-muted-foreground">
            {email[0]?.toUpperCase()}
          </div>
          <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground">
            {email}
          </span>
          <SignOutButton />
        </div>
      </AnimatedSidebarFooter>
    </AnimatedSidebar>
  );
}
