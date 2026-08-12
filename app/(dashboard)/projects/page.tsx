import { ProjectManager } from '@/components/projects/project-manager';
import { apiJSON } from '@/lib/api';
import type { ProjectsEnvelope } from '@/lib/projects';

export const dynamic = 'force-dynamic';

export default async function ProjectsPage() {
  let initialData: ProjectsEnvelope = { data: [], meta: { page: 1, per_page: 20, total: 0 } };
  try {
    initialData = await apiJSON<ProjectsEnvelope>('/api/v1/projects');
  } catch {
    // Client component will refetch and show its own error state.
  }
  return (
    <main
      id="dashboard-content"
      className="mx-auto flex w-full max-w-[1200px] scroll-mt-20 flex-col gap-8 px-6 py-12 sm:px-8 sm:py-16"
    >
      <header className="max-w-2xl">
        <p className="label-meta text-muted-foreground">Resources</p>
        <h1 className="heading-page mt-2">Projects</h1>
        <p className="mt-3 text-sm leading-6 text-muted-foreground">
          Projects are the foundation for services, immutable deployments, databases,
          and storage.
        </p>
      </header>
      <ProjectManager initialData={initialData} />
    </main>
  );
}
