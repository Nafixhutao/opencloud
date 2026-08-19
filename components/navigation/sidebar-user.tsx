'use client';

import { ChevronsUpDown, LogOut, UserCog } from 'lucide-react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useState } from 'react';

import { useAnimatedSidebarPanel } from '@/components/motion/animated-sidebar';
import { Avatar, AvatarFallback } from '@/components/ui/avatar';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLinkItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { authClient } from '@/lib/auth-client';

type SidebarUserProps = {
  email: string;
  role: string;
};

export function SidebarUser({ email, role }: SidebarUserProps) {
  const { collapsed } = useAnimatedSidebarPanel();
  const router = useRouter();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const initial = email.trim()[0]?.toUpperCase() ?? '?';

  async function onSignOut() {
    setError(null);
    setPending(true);
    try {
      const result = await authClient.signOut();
      if (result.error) {
        setError(result.error.message ?? 'Could not sign out. Try again.');
        return;
      }
      router.push('/login');
      router.refresh();
    } catch {
      setError('Could not reach Cevra. Check your connection and try again.');
    } finally {
      setPending(false);
    }
  }

  return (
    <>
      {error ? (
        <p role="alert" className="px-2 pb-1 text-xs text-destructive">
          {error}
        </p>
      ) : null}

      <DropdownMenu>
        <DropdownMenuTrigger
          aria-label={collapsed ? `Account menu for ${email}` : undefined}
          className="flex w-full min-w-0 items-center gap-2 rounded-xl px-1 py-1 text-left outline-none transition-colors hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Avatar className="size-7 rounded-full">
            <AvatarFallback className="rounded-full">{initial}</AvatarFallback>
          </Avatar>

          {collapsed ? null : (
            <>
              <span className="min-w-0 flex-1">
                <span className="block truncate text-xs font-medium text-sidebar-foreground">
                  {email}
                </span>
                <span className="block truncate text-[11px] text-muted-foreground">
                  {role}
                </span>
              </span>
              <ChevronsUpDown
                aria-hidden="true"
                className="size-3.5 shrink-0 text-muted-foreground"
              />
            </>
          )}
        </DropdownMenuTrigger>

        <DropdownMenuContent side="top" align="start" sideOffset={8}>
          <div className="px-2 py-1.5">
            <p className="truncate text-xs font-medium text-foreground">{email}</p>
            <p className="truncate text-[11px] text-muted-foreground">{role}</p>
          </div>
          <DropdownMenuSeparator />
          <DropdownMenuLinkItem render={<Link href="/account" />}>
            <UserCog />
            Account settings
          </DropdownMenuLinkItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            variant="destructive"
            disabled={pending}
            closeOnClick={false}
            onClick={onSignOut}
          >
            <LogOut />
            {pending ? 'Signing out…' : 'Sign out'}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </>
  );
}
