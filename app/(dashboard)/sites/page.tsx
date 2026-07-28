import { SiteDashboard } from '@/components/sites/site-dashboard';
import { apiJSON } from '@/lib/api';
import type { SitesEnvelope } from '@/lib/sites';

export default async function SitesPage() {
  const initialData = await apiJSON<SitesEnvelope>('/api/v1/sites');
  return (
    <main
      id="dashboard-content"
      className="mx-auto flex w-full max-w-[1200px] scroll-mt-20 flex-col gap-8 px-6 py-12 sm:px-8 sm:py-16"
    >
      <header className="max-w-2xl">
        <p className="label-meta text-muted-foreground">Resources</p>
        <h1 className="heading-page mt-2">Sites</h1>
        <p className="mt-3 text-sm leading-6 text-muted-foreground">
          Create, suspend, resume, and remove isolated site workloads. Lifecycle changes
          are durable jobs, so this page stays responsive while the worker acts.
        </p>
      </header>
      <SiteDashboard initialData={initialData} />
    </main>
  );
}
