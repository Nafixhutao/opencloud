'use client';

import { Button } from '@/components/ui/button';

export default function DashboardError({ reset }: { reset: () => void }) {
  return (
    <main className="flex min-h-svh w-full items-center justify-center bg-background p-6">
      <div className="w-full max-w-sm space-y-4 text-center">
        <h1 className="text-lg font-medium">Something went wrong</h1>
        <p className="text-sm text-muted-foreground">
          The dashboard could not be loaded. Try again, and contact support if the problem
          persists.
        </p>
        <Button className="h-11" onClick={reset}>
          Try again
        </Button>
      </div>
    </main>
  );
}
