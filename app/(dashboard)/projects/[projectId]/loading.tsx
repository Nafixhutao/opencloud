import { Skeleton } from '@/components/ui/skeleton';

export default function ProjectDetailLoading() {
  return (
    <main
      id="dashboard-content"
      className="mx-auto flex w-full max-w-[1200px] flex-col gap-8 px-6 py-12 sm:px-8 sm:py-16"
      aria-label="Loading project details"
    >
      <div className="flex items-center justify-between gap-4">
        <div className="space-y-2">
          <Skeleton className="h-5 w-20" />
          <Skeleton className="h-8 w-48" />
        </div>
        <Skeleton className="h-9 w-32 rounded-md" />
      </div>
      <Skeleton className="h-40 w-full rounded-xl" />
      <Skeleton className="h-72 w-full rounded-xl" />
    </main>
  );
}
