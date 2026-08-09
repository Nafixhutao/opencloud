'use client';

import { Button } from '@/components/ui/button';

export default function ProjectsError({ reset }: { reset: () => void }) {
  return <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-4 px-6 py-12 sm:px-8 sm:py-16"><h1 className="heading-page">Projects are temporarily unavailable</h1><p className="text-sm text-muted-foreground">Try again in a moment. If this continues, contact support.</p><div><Button onClick={reset}>Try again</Button></div></main>;
}
