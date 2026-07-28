import { DatabaseDashboard } from '@/components/databases/database-dashboard';
import { apiJSON } from '@/lib/api';
import type { DatabasesEnvelope } from '@/lib/databases';

export default async function DatabasesPage() {
  const initialData = await apiJSON<DatabasesEnvelope>('/api/v1/databases');
  return (
    <main
      id="dashboard-content"
      className="mx-auto flex w-full max-w-[1200px] scroll-mt-20 flex-col gap-8 px-6 py-12 sm:px-8 sm:py-16"
    >
      <header className="max-w-2xl">
        <p className="label-meta text-muted-foreground">Resources</p>
        <h1 className="heading-page mt-2">Databases</h1>
        <p className="mt-3 text-sm leading-6 text-muted-foreground">
          Provision a scoped PostgreSQL or MariaDB database. Credentials are encrypted
          until you reveal them, then permanently removed from the control plane.
        </p>
      </header>
      <DatabaseDashboard initialData={initialData} />
    </main>
  );
}
