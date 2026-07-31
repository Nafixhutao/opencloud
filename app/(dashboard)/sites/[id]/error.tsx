'use client';

import { CircleAlert, RefreshCw } from 'lucide-react';
import Link from 'next/link';
import { useEffect } from 'react';

import { Button } from '@/components/ui/button';
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty';

export default function SiteDomainError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error(error);
  }, [error]);

  return (
    <main
      id="dashboard-content"
      className="mx-auto flex min-h-[70svh] w-full max-w-[1200px] items-center px-6 py-12 sm:px-8"
    >
      <Empty className="min-h-72 border">
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <CircleAlert aria-hidden="true" />
          </EmptyMedia>
          <EmptyTitle>Could not load site domains</EmptyTitle>
          <EmptyDescription>
            The control plane did not return this site’s domain state. Retry, or return to
            the sites list.
          </EmptyDescription>
        </EmptyHeader>
        <EmptyContent className="flex-row justify-center">
          <Button type="button" onClick={reset}>
            <RefreshCw data-icon="inline-start" />
            Retry
          </Button>
          <Button variant="outline" render={<Link href="/sites" />} nativeButton={false}>
            Back to sites
          </Button>
        </EmptyContent>
      </Empty>
    </main>
  );
}
