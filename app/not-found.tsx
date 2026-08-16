import { NotFoundStacked } from '@/components/motion/not-found/stacked';

export default function NotFound() {
  return (
    <main className="flex min-h-svh items-center justify-center px-4 py-16">
      <NotFoundStacked
        homeHref="/dashboard"
        homeLabel="Back to dashboard"
        browseHref="/sites"
        browseLabel="View your sites"
      />
    </main>
  );
}
