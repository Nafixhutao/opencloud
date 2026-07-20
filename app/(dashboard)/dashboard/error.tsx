'use client';

import { Button } from '@/components/ui/button';

export default function DashboardError({ reset }: { reset: () => void }) {
  return (
    <main className="mx-auto flex min-h-[calc(100svh-4rem)] w-full max-w-[1200px] items-center px-6 py-16 sm:px-8">
      <div className="flex w-full max-w-2xl flex-col items-start gap-5">
        <p className="label-meta text-destructive">Dashboard Unavailable</p>
        <h1 className="heading-page">The Dashboard Didn&apos;t Load</h1>
        <p className="max-w-lg text-sm leading-6 text-muted-foreground">
          The workspace could not be loaded. Retry the dashboard, then contact support if
          the problem continues.
        </p>
        <Button onClick={reset}>Retry Dashboard</Button>
      </div>
    </main>
  );
}
