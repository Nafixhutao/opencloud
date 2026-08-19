'use client';

import { PanelLeft } from 'lucide-react';

import {
  AnimatedSidebarTrigger,
  useAnimatedSidebarPanel,
} from '@/components/motion/animated-sidebar';
import { Avatar, AvatarFallback } from '@/components/ui/avatar';
import { Badge } from '@/components/ui/badge';

type SidebarWorkspaceProps = {
  name: string;
  plan?: string;
};

/**
 * Workspace identity row: brand mark, workspace name, plan badge, and the
 * panel toggle. Collapses to the bare mark at icon width.
 */
export function SidebarWorkspace({ name, plan }: SidebarWorkspaceProps) {
  const { collapsed } = useAnimatedSidebarPanel();

  const mark = (
    <Avatar className="size-8 rounded-md bg-primary">
      <AvatarFallback className="bg-primary text-sm font-medium tracking-tight text-primary-foreground">
        OC
      </AvatarFallback>
    </Avatar>
  );

  // Collapsed, the brand mark doubles as the expand control. The rail hairline
  // is tabIndex=-1, so without this there would be no keyboard-reachable way
  // back to the expanded panel other than the Ctrl/Cmd+B shortcut.
  if (collapsed) {
    return (
      <div className="flex h-10 items-center justify-center">
        <AnimatedSidebarTrigger
          aria-label="Expand sidebar"
          className="group/mark relative grid size-8 place-items-center rounded-md outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <span className="transition-opacity group-hover/mark:opacity-0">
            {mark}
          </span>
          <PanelLeft
            aria-hidden="true"
            className="absolute size-4 text-muted-foreground opacity-0 transition-opacity group-hover/mark:opacity-100"
          />
        </AnimatedSidebarTrigger>
      </div>
    );
  }

  return (
    <div className="flex h-10 items-center gap-2 px-1">
      {mark}

      <div className="flex min-w-0 flex-1 items-center gap-1.5">
        <span className="truncate text-sm font-medium tracking-tight text-sidebar-foreground">
          {name}
        </span>
        {plan ? (
          <Badge variant="secondary" className="shrink-0">
            {plan}
          </Badge>
        ) : null}
      </div>

      <AnimatedSidebarTrigger
        aria-label="Collapse sidebar"
        className="grid size-7 shrink-0 place-items-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
      >
        <PanelLeft aria-hidden="true" className="size-4" />
      </AnimatedSidebarTrigger>
    </div>
  );
}
