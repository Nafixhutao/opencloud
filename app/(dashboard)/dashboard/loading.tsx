export default function DashboardLoading() {
  return (
    <main className="flex min-h-svh w-full items-center justify-center bg-background p-6">
      <div className="w-full max-w-sm space-y-3" aria-busy="true" aria-label="Loading dashboard">
        <div className="h-32 animate-pulse rounded-xl bg-muted motion-reduce:animate-none" />
        <div className="h-8 w-24 animate-pulse rounded-lg bg-muted motion-reduce:animate-none" />
      </div>
    </main>
  );
}
