import { Skeleton } from '@/components/ui/skeleton';

export default function SiteDomainLoading() {
  return (
    <main
      id="dashboard-content"
      className="mx-auto flex w-full max-w-[1200px] flex-col gap-8 px-6 py-12 sm:px-8 sm:py-16"
      aria-label="Loading site domains"
    >
      <div className="space-y-3">
        <Skeleton className="h-5 w-24" />
        <Skeleton className="h-10 w-full max-w-lg" />
        <Skeleton className="h-5 w-full max-w-2xl" />
      </div>
      <Skeleton className="h-72 w-full" />
      <Skeleton className="h-80 w-full" />
    </main>
  );
}
