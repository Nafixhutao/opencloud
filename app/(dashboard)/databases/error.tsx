'use client';

import { Database } from 'lucide-react';

import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty';
import { Button } from '@/components/ui/button';

export default function DatabasesError({ reset }: { reset: () => void }) {
  return (
    <main
      id="dashboard-content"
      className="mx-auto flex min-h-[70svh] w-full max-w-[1200px] items-center px-6 py-12 sm:px-8"
    >
      <Empty className="min-h-72 border">
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <Database aria-hidden="true" />
          </EmptyMedia>
          <EmptyTitle>Databases are temporarily unavailable</EmptyTitle>
          <EmptyDescription>
            The dashboard could not load database state from the control plane.
          </EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button type="button" onClick={reset}>
            Try again
          </Button>
        </EmptyContent>
      </Empty>
    </main>
  );
}
