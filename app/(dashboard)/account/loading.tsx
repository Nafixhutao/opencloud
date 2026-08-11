import { Skeleton } from '@/components/ui/skeleton';

export default function AccountLoading() {
  return (
    <main
      id="dashboard-content"
      className="mx-auto flex w-full max-w-[800px] scroll-mt-20 flex-col gap-10 px-6 py-12 sm:px-8 sm:py-16"
      aria-label="Loading account settings"
    >
      <header className="flex flex-col gap-2">
        <Skeleton className="h-5 w-20" />
        <Skeleton className="h-9 w-48" />
        <Skeleton className="h-5 w-full max-w-2xl" />
      </header>
      <Skeleton className="h-72 w-full rounded-xl" />
      <Skeleton className="h-56 w-full rounded-xl" />
    </main>
  );
}
