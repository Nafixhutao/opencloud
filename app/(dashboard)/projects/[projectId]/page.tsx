import Link from 'next/link';
import { notFound } from 'next/navigation';
import { GitBranchIcon, FolderIcon } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { ProjectLogsViewer } from '@/components/projects/project-logs-viewer';
import { apiJSON } from '@/lib/api';
import type { Project, ProjectServicesEnvelope } from '@/lib/projects';

type ProjectPageProps = { params: Promise<{ projectId: string }> };

export default async function ProjectPage({ params }: ProjectPageProps) {
  const { projectId } = await params;
  const [projectResult, servicesResult] = await Promise.allSettled([
    apiJSON<{ data: Project }>(`/api/v1/projects/${projectId}`),
    apiJSON<ProjectServicesEnvelope>(`/api/v1/projects/${projectId}/services`),
  ]);
  if (projectResult.status === 'rejected') {
    const status = (projectResult.reason as { status?: number }).status;
    if (status === 404) notFound();
    throw projectResult.reason;
  }
  if (servicesResult.status === 'rejected') throw servicesResult.reason;
  const project = projectResult.value.data;
  const services = servicesResult.value.data;

  return <main id="dashboard-content" className="mx-auto flex w-full max-w-[1200px] scroll-mt-20 flex-col gap-8 px-6 py-12 sm:px-8 sm:py-16"><header><Button render={<Link href="/projects" />} nativeButton={false} size="sm" variant="ghost">← Projects</Button><p className="label-meta mt-6 text-muted-foreground">Project</p><h1 className="heading-page mt-2">{project.name}</h1><p className="mt-3 text-sm text-muted-foreground">Inspect every service and follow tenant-scoped deployment activity in real time.</p></header><Card><CardHeader><CardTitle>Services</CardTitle><CardDescription>{services.length === 0 ? 'No services have been added yet.' : 'Each service has an independent build, runtime, request, and platform log scope.'}</CardDescription></CardHeader>{services.length > 0 ? <CardContent><ul className="divide-y rounded-lg border">{services.map((service) => <li key={service.id} className="flex flex-col gap-1 px-4 py-3 sm:flex-row sm:items-center sm:justify-between"><div><span className="font-medium">{service.name}</span><div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">{service.source_root ? <span className="inline-flex items-center gap-1"><FolderIcon size={12} />{service.source_root}</span> : null}{service.git_repo_url ? <span className="inline-flex items-center gap-1"><GitBranchIcon size={12} />{service.git_repo_url} ({service.git_branch})</span> : null}</div></div><span className="text-sm text-muted-foreground">{service.type} · {service.status}</span></li>)}</ul></CardContent> : null}</Card><ProjectLogsViewer projectId={project.id} services={services} /></main>;
}
