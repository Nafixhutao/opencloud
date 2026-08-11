import { Skeleton } from '@/components/ui/skeleton';

export default function DatabaseDetailLoading() {
  return (
    <main
      id="dashboard-content"
      className="mx-auto flex w-full max-w-[1200px] scroll-mt-20 flex-col gap-8 px-6 py-12 sm:px-8 sm:py-16"
      aria-label="Loading database console"
    >
      <header className="max-w-2xl">
        <Skeleton className="h-5 w-24" />
        <Skeleton className="mt-2 h-9 w-48" />
        <Skeleton className="mt-3 h-5 w-full" />
      </header>
      <Skeleton className="h-64 w-full rounded-xl" />
      <Skeleton className="h-96 w-full rounded-xl" />
    </main>
  );
}
