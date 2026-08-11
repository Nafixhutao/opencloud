'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Boxes, Plus } from 'lucide-react';
import Link from 'next/link';
import { useEffect, useState } from 'react';
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
  const [error, setError] = useState<string | null>(null);
  const form = useForm<CreateProjectValues>({
    resolver: zodResolver(createProjectSchema),
    defaultValues: { name: '' },
  });

  useEffect(() => {
    // No error state in initialData, just log it if something goes wrong
    console.log('Projects loaded:', initialData);
  }, [initialData]);

  const projects = useQuery({
    queryKey: ['projects'],
    queryFn: listProjects,
    initialData,
    staleTime: 0,
  });

  const create = useMutation({
    mutationFn: ({ name, key }: { name: string; key: string }) => createProject(name, key),
    onSuccess: async () => {
      form.reset();
      await queryClient.invalidateQueries({ queryKey: ['projects'] });
      setError(null);
    },
    onError: (err) => {
      if (err instanceof ProjectAPIError) {
        setError(err.message);
      } else {
        setError('Failed to create project. Please try again.');
      }
    },
  });

  async function onSubmit(values: CreateProjectValues) {
    setError(null);
    try {
      await create.mutateAsync({ name: values.name, key: crypto.randomUUID() });
    } catch {
      // Error already handled by mutation
    }
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
          {error && <FieldError>{error}</FieldError>}
          <form onSubmit={form.handleSubmit(onSubmit)} noValidate>
            <FieldGroup className="max-w-xl gap-4">
              <Field data-invalid={Boolean(form.formState.errors.name)}>
                <FieldLabel htmlFor="project-name">Project name</FieldLabel>
                <Input 
                  id="project-name" 
                  placeholder="toko-online" 
                  aria-invalid={Boolean(form.formState.errors.name)} 
                  {...form.register('name')} 
                />
                <FieldError errors={[form.formState.errors.name]} />
              </Field>
              <Button 
                type="submit" 
                disabled={create.isPending || form.formState.isSubmitting} 
                aria-busy={create.isPending || form.formState.isSubmitting}
                className="w-full"
              >
                {create.isPending || form.formState.isSubmitting ? (
                  <>
                    <Spinner data-icon="inline-start" /> Creating…
                  </>
                ) : (
                  <>
                    <Plus data-icon="inline-start" /> Create project
                  </>
                )}
              </Button>
            </FieldGroup>
          </form>
        </CardContent>
      </Card>

      <section aria-labelledby="projects-heading" className="flex flex-col gap-4">
        <div className="flex items-end justify-between gap-4">
          <div>
            <p className="label-meta text-muted-foreground">Application platform</p>
            <h2 id="projects-heading" className="heading-section mt-1">Projects</h2>
          </div>
          <p className="text-sm text-muted-foreground tabular-nums">
            {projects.data?.meta?.total || 0} {projects.data?.meta?.total === 1 ? 'project' : 'projects'}
          </p>
        </div>
        
        {projects.isLoading ? (
          <div className="flex items-center justify-center py-16 border rounded-lg">
            <Spinner /> Loading…
          </div>
        ) : projects.data?.data.length === 0 ? (
          <Empty className="min-h-64 border">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <Boxes aria-hidden="true" />
              </EmptyMedia>
              <EmptyTitle>No projects yet</EmptyTitle>
              <EmptyDescription>Create a project to begin organizing services and deployment history.</EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          <div className="grid gap-4 sm:grid-cols-2">
            {projects.data?.data.map((project) => (
              <Link 
                key={project.id} 
                href={`/projects/${project.id}`} 
                className="rounded-lg border p-5 transition-colors hover:bg-secondary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <p className="font-medium">{project.name}</p>
                <p className="mt-2 text-sm text-muted-foreground">
                  {project.status === 'active' ? 'Ready for services' : project.status}
                </p>
              </Link>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
