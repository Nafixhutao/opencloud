'use client';

import { ChevronDown, Search } from 'lucide-react';
import { useMemo, useState } from 'react';

import {
  AnimatedSidebar,
  AnimatedSidebarContent,
  AnimatedSidebarFooter,
  AnimatedSidebarGroup,
  AnimatedSidebarGroupContent,
  AnimatedSidebarGroupLabel,
  AnimatedSidebarHeader,
  useAnimatedSidebarPanel,
} from '@/components/motion/animated-sidebar';
import { NavMain } from '@/components/navigation/nav-main';
import { SidebarUser } from '@/components/navigation/sidebar-user';
import { SidebarWorkspace } from '@/components/navigation/sidebar-workspace';
import { buildNav, filterNavItems } from '@/lib/navigation';
import { cn } from '@/lib/utils';

type SidebarProps = {
  email: string;
  isAdmin: boolean;
  plan?: string;
};

// App navigation on the beUI animated-sidebar primitives: icon-rail collapse,
// spring active pill, built-in mobile sheet, and Ctrl/Cmd+B toggle.
export function Sidebar({ email, isAdmin, plan }: SidebarProps) {
  const [query, setQuery] = useState('');
  const [showSecondary, setShowSecondary] = useState(false);

  const groups = useMemo(() => buildNav({ isAdmin }), [isAdmin]);
  const searching = query.trim().length > 0;

  const visibleGroups = useMemo(
    () =>
      groups
        .filter((group) => searching || showSecondary || !group.secondary)
        .map((group) => ({ ...group, items: filterNavItems(group.items, query) }))
        .filter((group) => group.items.length > 0),
    [groups, query, searching, showSecondary],
  );

  const hasSecondary = groups.some((group) => group.secondary);

  return (
    <AnimatedSidebar
      ariaLabel="OpenCloud navigation"
      collapsible="icon"
      panelClassName="bg-sidebar text-sidebar-foreground border-sidebar-border"
    >
      <AnimatedSidebarHeader>
        <SidebarWorkspace name="OpenCloud" plan={plan} />
      </AnimatedSidebarHeader>

      <AnimatedSidebarContent>
        <NavSearch value={query} onChange={setQuery} />

        {visibleGroups.map((group) => (
          <AnimatedSidebarGroup key={group.id}>
            {group.label ? (
              <AnimatedSidebarGroupLabel>{group.label}</AnimatedSidebarGroupLabel>
            ) : null}
            <AnimatedSidebarGroupContent>
              <NavMain items={group.items} />
            </AnimatedSidebarGroupContent>
          </AnimatedSidebarGroup>
        ))}

        {visibleGroups.length === 0 ? (
          <p className="px-3 py-2 text-xs text-muted-foreground">
            No navigation matches “{query.trim()}”.
          </p>
        ) : null}

        {hasSecondary && !searching ? (
          <ViewMoreToggle
            expanded={showSecondary}
            onToggle={() => setShowSecondary((value) => !value)}
          />
        ) : null}
      </AnimatedSidebarContent>

      <AnimatedSidebarFooter>
        <SidebarUser email={email} role={isAdmin ? 'Admin' : 'Member'} />
      </AnimatedSidebarFooter>
    </AnimatedSidebar>
  );
}

/** Filters the nav by label. Collapses to a static icon at rail width. */
function NavSearch({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  const { collapsed } = useAnimatedSidebarPanel();

  if (collapsed) {
    return (
      <div
        aria-hidden="true"
        className="grid h-9 shrink-0 place-items-center text-muted-foreground"
      >
        <Search className="size-4" />
      </div>
    );
  }

  return (
    <div className="relative shrink-0 px-1">
      <Search
        aria-hidden="true"
        className="pointer-events-none absolute top-1/2 left-3.5 size-4 -translate-y-1/2 text-muted-foreground"
      />
      <input
        type="search"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder="Search"
        aria-label="Search navigation"
        className="h-9 w-full rounded-xl bg-transparent pr-3 pl-9 text-sm text-foreground outline-none transition-colors placeholder:text-muted-foreground hover:bg-muted/60 focus-visible:bg-muted/70 focus-visible:ring-2 focus-visible:ring-ring"
      />
    </div>
  );
}

/** Floating pill that reveals or hides the secondary nav groups. */
function ViewMoreToggle({
  expanded,
  onToggle,
}: {
  expanded: boolean;
  onToggle: () => void;
}) {
  const { collapsed } = useAnimatedSidebarPanel();
  if (collapsed) return null;

  return (
    <div className="flex shrink-0 justify-center py-1">
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={expanded}
        className="inline-flex items-center gap-1.5 rounded-full border border-border bg-card px-2.5 py-1 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
      >
        <ChevronDown
          aria-hidden="true"
          className={cn(
            'size-3.5 transition-transform duration-200',
            expanded && 'rotate-180',
          )}
        />
        {expanded ? 'View less' : 'View more'}
      </button>
    </div>
  );
}
