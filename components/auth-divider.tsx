import type React from 'react';

import { cn } from '@/lib/utils';

export function AuthDivider({
  children,
  className,
  ...props
}: React.ComponentProps<'div'>) {
  return (
    <div className={cn('relative flex w-full items-center', className)} {...props}>
      <div className="w-full border-t" />
      <div className="flex w-max justify-center px-3 text-xs text-nowrap text-muted-foreground">
        {children}
      </div>
      <div className="w-full border-t" />
    </div>
  );
}
