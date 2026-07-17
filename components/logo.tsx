import { Cloud } from 'lucide-react';
import type { ComponentProps } from 'react';

import { cn } from '@/lib/utils';

export function Logo({ className, ...props }: ComponentProps<'div'>) {
  return (
    <div
      className={cn('flex items-center gap-2.5 text-sm font-semibold tracking-[-0.02em]', className)}
      {...props}
    >
      <span className="flex size-8 items-center justify-center rounded-lg bg-foreground text-background shadow-sm">
        <Cloud className="size-4.5" strokeWidth={2.2} />
      </span>
      <span>OpenCloud</span>
    </div>
  );
}
