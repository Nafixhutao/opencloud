import { Skeleton } from '@/components/ui/skeleton';

export default function AdminUsersLoading() {
  return (
    <main
      id="dashboard-content"
      className="mx-auto flex w-full max-w-[1200px] scroll-mt-20 flex-col gap-10 px-6 py-12 sm:px-8 sm:py-16"
      aria-label="Loading admin users"
    >
      <header className="flex flex-col gap-2">
        <Skeleton className="h-5 w-24" />
        <Skeleton className="h-9 w-40" />
        <Skeleton className="h-5 w-full max-w-2xl" />
      </header>
      <Skeleton className="h-96 w-full rounded-lg" />
    </main>
  );
}
