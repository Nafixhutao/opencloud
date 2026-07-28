import { Skeleton } from '@/components/ui/skeleton';

export default function DatabasesLoading() {
  return (
    <main
      id="dashboard-content"
      className="mx-auto flex w-full max-w-[1200px] flex-col gap-8 px-6 py-12 sm:px-8 sm:py-16"
    >
      <div className="flex max-w-2xl flex-col gap-3">
        <Skeleton className="h-3 w-24" />
        <Skeleton className="h-9 w-52" />
        <Skeleton className="h-5 w-full" />
      </div>
      <Skeleton className="h-72 w-full rounded-xl" />
      <Skeleton className="h-56 w-full rounded-xl" />
    </main>
  );
}
