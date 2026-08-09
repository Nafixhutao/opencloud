'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Boxes, Plus } from 'lucide-react';
import Link from 'next/link';
import { useForm } from 'react-hook-form';

import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty';
import { Field, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { Spinner } from '@/components/ui/spinner';
import { createProject, listProjects, ProjectAPIError, type ProjectsEnvelope } from '@/lib/projects';
import { createProjectSchema, type CreateProjectValues } from '@/lib/project-validation';

export function ProjectDashboard({ initialData }: { initialData: ProjectsEnvelope }) {
  const queryClient = useQueryClient();
  const form = useForm<CreateProjectValues>({
    resolver: zodResolver(createProjectSchema),
    defaultValues: { name: '' },
  });
  const projects = useQuery({ queryKey: ['projects'], queryFn: listProjects, initialData });
  const create = useMutation({
    mutationFn: ({ name, key }: { name: string; key: string }) => createProject(name, key),
    onSuccess: async () => {
      form.reset();
      await queryClient.invalidateQueries({ queryKey: ['projects'] });
    },
  });
  const error = create.error instanceof ProjectAPIError
    ? create.error.message
    : create.error
      ? 'The control plane could not complete the request.'
      : null;

  async function onSubmit(values: CreateProjectValues) {
    await create.mutateAsync({ name: values.name, key: crypto.randomUUID() });
  }

  return (
    <div className="flex flex-col gap-10">
      <Card>
        <CardHeader>
          <CardTitle>Create a project</CardTitle>
          <CardDescription>
            Group the services that belong to one application. Source connection and deployments land in later phases.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={form.handleSubmit(onSubmit)} noValidate>
            <FieldGroup className="max-w-xl gap-4">
              {error ? <FieldError>{error}</FieldError> : null}
              <Field data-invalid={Boolean(form.formState.errors.name)}>
                <FieldLabel htmlFor="project-name">Project name</FieldLabel>
                <Input id="project-name" placeholder="toko-online" aria-invalid={Boolean(form.formState.errors.name)} {...form.register('name')} />
                <FieldError errors={[form.formState.errors.name]} />
              </Field>
              <div><Button type="submit" disabled={create.isPending} aria-busy={create.isPending}>{create.isPending ? <Spinner data-icon="inline-start" /> : <Plus data-icon="inline-start" />}{create.isPending ? 'Creating…' : 'Create project'}</Button></div>
            </FieldGroup>
          </form>
        </CardContent>
      </Card>

      <section aria-labelledby="projects-heading" className="flex flex-col gap-4">
        <div className="flex items-end justify-between gap-4"><div><p className="label-meta text-muted-foreground">Application platform</p><h2 id="projects-heading" className="heading-section mt-1">Projects</h2></div><p className="text-sm text-muted-foreground tabular-nums">{projects.data.meta.total} {projects.data.meta.total === 1 ? 'project' : 'projects'}</p></div>
        {projects.data.data.length === 0 ? (
          <Empty className="min-h-64 border"><EmptyHeader><EmptyMedia variant="icon"><Boxes aria-hidden="true" /></EmptyMedia><EmptyTitle>No projects yet</EmptyTitle><EmptyDescription>Create a project to begin organizing services and deployment history.</EmptyDescription></EmptyHeader></Empty>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2">{projects.data.data.map((project) => <Link key={project.id} href={`/projects/${project.id}`} className="rounded-lg border p-5 transition-colors hover:bg-secondary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"><p className="font-medium">{project.name}</p><p className="mt-2 text-sm text-muted-foreground">{project.status === 'active' ? 'Ready for services' : project.status}</p></Link>)}</div>
        )}
      </section>
    </div>
  );
}
