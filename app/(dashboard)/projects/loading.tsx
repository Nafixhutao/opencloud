import { Skeleton } from '@/components/ui/skeleton';

export default function Loading() {
  return <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-8 px-6 py-12 sm:px-8 sm:py-16"><Skeleton className="h-8 w-44" /><Skeleton className="h-48 w-full" /><Skeleton className="h-32 w-full" /></main>;
}
