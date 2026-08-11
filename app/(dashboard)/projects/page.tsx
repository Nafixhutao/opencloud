import { ProjectDashboard } from '@/components/projects/project-dashboard';
import { apiJSON } from '@/lib/api';
import type { ProjectsEnvelope } from '@/lib/projects';

export default async function ProjectsPage() {
  let initialData: ProjectsEnvelope = { data: [], meta: { page: 1, per_page: 20, total: 0 } };
  try {
    initialData = await apiJSON<ProjectsEnvelope>('/api/v1/projects');
  } catch (error) {
    console.error('Failed to load projects:', error);
  }
  return <main id="dashboard-content" className="mx-auto flex w-full max-w-[1200px] scroll-mt-20 flex-col gap-8 px-6 py-12 sm:px-8 sm:py-16"><header className="max-w-2xl"><p className="label-meta text-muted-foreground">Resources</p><h1 className="heading-page mt-2">Projects</h1><p className="mt-3 text-sm leading-6 text-muted-foreground">Projects are the foundation for services, immutable deployments, databases, and storage.</p></header><ProjectDashboard initialData={initialData} /></main>;
}
