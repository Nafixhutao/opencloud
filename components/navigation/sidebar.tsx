'use client';

import { useState } from 'react';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import {
  LayoutDashboard, FolderKanban, Database, Globe,
  UserCog, Users, Menu, X, ChevronLeft,
} from 'lucide-react';

import { SignOutButton } from '@/components/auth/sign-out-button';

type SidebarProps = {
  email: string;
  isAdmin: boolean;
};

export function Sidebar({ email, isAdmin }: SidebarProps) {
  const pathname = usePathname();
  const [collapsed, setCollapsed] = useState(false);
  const [mobileOpen, setMobileOpen] = useState(false);

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

  const sidebar = (
    <div className={`flex h-full flex-col bg-sidebar text-sidebar-foreground transition-all ${collapsed ? 'w-16' : 'w-56'}`}>
      {/* Logo */}
      <div className="flex h-14 items-center gap-3 border-b border-sidebar-border px-3">
        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-primary text-sm font-medium tracking-tight text-primary-foreground">
          OC
        </div>
        {!collapsed && <span className="text-sm font-medium tracking-tight text-sidebar-foreground">OpenCloud</span>}
        <button
          onClick={() => setCollapsed(!collapsed)}
          className="ml-auto hidden rounded p-1 text-muted-foreground hover:text-sidebar-foreground lg:block"
          aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
        >
          <ChevronLeft size={16} className={`transition-transform ${collapsed ? 'rotate-180' : ''}`} />
        </button>
      </div>

      {/* Nav */}
      <nav className="flex-1 space-y-0.5 overflow-y-auto px-2 py-3">
        {links.map(({ href, label, icon: Icon }) => (
          <Link
            key={href}
            href={href}
            onClick={() => setMobileOpen(false)}
            className={`flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors ${
              isActive(href)
                ? 'bg-foreground/10 text-foreground'
                : 'text-muted-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground'
            }`}
          >
            <Icon size={18} className="shrink-0" />
            {!collapsed && <span>{label}</span>}
          </Link>
        ))}
      </nav>

      {/* User */}
      <div className="border-t border-sidebar-border p-3">
        <div className="flex items-center gap-3">
          <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-muted text-xs text-muted-foreground">
            {email[0]?.toUpperCase()}
          </div>
          {!collapsed && (
            <div className="min-w-0 flex-1">
              <p className="truncate text-xs text-muted-foreground">{email}</p>
            </div>
          )}
          <SignOutButton />
        </div>
      </div>
    </div>
  );

  return (
    <>
      {/* Mobile hamburger */}
      <button
        onClick={() => setMobileOpen(!mobileOpen)}
        className="fixed left-3 top-3 z-50 rounded-md bg-popover p-2 text-popover-foreground ring-1 ring-border lg:hidden"
        aria-label="Toggle menu"
      >
        {mobileOpen ? <X size={20} /> : <Menu size={20} />}
      </button>

      {/* Overlay */}
      {mobileOpen && (
        <div
          className="fixed inset-0 z-40 bg-black/50 lg:hidden"
          onClick={() => setMobileOpen(false)}
        />
      )}

      {/* Desktop sidebar */}
      <aside className="hidden shrink-0 lg:block">{sidebar}</aside>

      {/* Mobile sidebar */}
      <aside
        className={`fixed inset-y-0 left-0 z-40 transition-transform lg:hidden ${
          mobileOpen ? 'translate-x-0' : '-translate-x-full'
        }`}
      >
        {sidebar}
      </aside>
    </>
  );
}
