'use client';

import { CircleAlert, RotateCcw } from 'lucide-react';

import { Button } from '@/components/ui/button';
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty';

export default function SitesError({ reset }: { reset: () => void }) {
  return (
    <main
      id="dashboard-content"
      className="mx-auto flex min-h-[60svh] w-full max-w-[1200px] items-center px-6 py-12 sm:px-8"
    >
      <Empty className="border">
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <CircleAlert aria-hidden="true" />
          </EmptyMedia>
          <EmptyTitle>Sites are temporarily unavailable</EmptyTitle>
          <EmptyDescription>
            The control plane did not return the site list. Retry, then check service
            health if the problem continues.
          </EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button type="button" variant="outline" onClick={reset}>
            <RotateCcw data-icon="inline-start" />
            Retry
          </Button>
        </EmptyContent>
      </Empty>
    </main>
  );
}
