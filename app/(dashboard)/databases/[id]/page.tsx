import { apiJSON } from '@/lib/api';
import type { ManagedDatabase } from '@/lib/databases';
import { DatabaseConsole } from '@/components/databases/database-console';

type DatabaseEnvelope = { data: ManagedDatabase };

type PageContext = { params: Promise<{ id: string }> };

export default async function DatabaseConsolePage({ params }: PageContext) {
  const { id } = await params;
  const { data: database } = await apiJSON<DatabaseEnvelope>(`/api/databases/${id}`);
  return (
    <main
      id="dashboard-content"
      className="mx-auto flex w-full max-w-[1200px] scroll-mt-20 flex-col gap-8 px-6 py-12 sm:px-8 sm:py-16"
    >
      <header className="max-w-2xl">
        <p className="label-meta text-muted-foreground">Resources</p>
        <h1 className="heading-page mt-2">{database.name}</h1>
        <p className="mt-3 text-sm leading-6 text-muted-foreground">
          Run read-only SQL against your scoped database. Every query is audited
          and bounded to keep your data safe.
        </p>
      </header>
      <DatabaseConsole database={database} />
    </main>
  );
}
