import { Skeleton } from '@/components/ui/skeleton';

export default function DashboardLoading() {
  return (
    <main
      className="mx-auto flex w-full max-w-[1200px] flex-col gap-16 px-6 py-12 sm:px-8 sm:py-16"
      aria-busy="true"
      aria-label="Loading dashboard"
    >
      <div className="flex flex-col gap-4">
        <Skeleton className="h-4 w-32" />
        <Skeleton className="h-14 w-full max-w-xl" />
        <Skeleton className="h-6 w-full max-w-2xl" />
      </div>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 4 }, (_, index) => (
          <Skeleton key={index} className="h-32" />
        ))}
      </div>
      <div className="grid gap-5 lg:grid-cols-12">
        <Skeleton className="h-[32rem] lg:col-span-8" />
        <Skeleton className="h-[32rem] lg:col-span-4" />
      </div>
    </main>
  );
}
