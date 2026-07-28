'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  CircleAlert,
  CircleCheck,
  Clock3,
  Pause,
  Play,
  Plus,
  Server,
  Trash2,
} from 'lucide-react';
import { useState } from 'react';
import { useForm } from 'react-hook-form';

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty';
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Input } from '@/components/ui/input';
import { Spinner } from '@/components/ui/spinner';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  createSite,
  deleteSite,
  hasPendingSites,
  listSites,
  resumeSite,
  SiteAPIError,
  type Site,
  type SitesEnvelope,
  suspendSite,
} from '@/lib/sites';
import { createSiteFormSchema, type CreateSiteValues } from '@/lib/site-validation';

type SiteDashboardProps = {
  initialData: SitesEnvelope;
};

type LifecycleAction = 'suspend' | 'resume' | 'delete';

const statusLabel: Record<Site['status'], string> = {
  provisioning: 'Provisioning',
  active: 'Active',
  suspending: 'Suspending',
  suspended: 'Suspended',
  resuming: 'Resuming',
  deleting: 'Deleting',
  deleted: 'Deleted',
  failed: 'Failed',
};

export function SiteDashboard({ initialData }: SiteDashboardProps) {
  const queryClient = useQueryClient();
  const form = useForm<CreateSiteValues>({
    resolver: zodResolver(createSiteFormSchema),
    defaultValues: { domain: '' },
  });
  const sitesQuery = useQuery({
    queryKey: ['sites'],
    queryFn: listSites,
    initialData,
    refetchInterval: (query) =>
      query.state.data && hasPendingSites(query.state.data.data) ? 2_000 : false,
  });

  const createMutation = useMutation({
    mutationFn: ({ domain, key }: { domain: string; key: string }) => createSite(domain, key),
    onSuccess: async () => {
      form.reset();
      await queryClient.invalidateQueries({ queryKey: ['sites'] });
    },
  });
  const lifecycleMutation = useMutation({
    mutationFn: ({ siteID, action }: { siteID: string; action: LifecycleAction }) => {
      if (action === 'suspend') {
        return suspendSite(siteID);
      }
      if (action === 'resume') {
        return resumeSite(siteID);
      }
      return deleteSite(siteID);
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['sites'] });
    },
  });

  async function onCreate(values: CreateSiteValues) {
    await createMutation.mutateAsync({ domain: values.domain, key: crypto.randomUUID() });
  }

  const mutationError = createMutation.error ?? lifecycleMutation.error;
  const errorMessage =
    mutationError instanceof SiteAPIError
      ? mutationError.message
      : mutationError
        ? 'The control plane could not complete the request.'
        : null;

  return (
    <div className="flex flex-col gap-10">
      <Card>
        <CardHeader>
          <CardTitle>Create a site</CardTitle>
          <CardDescription>
            Start with the curated static runtime. The worker will place it on available
            capacity and publish it asynchronously.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={form.handleSubmit(onCreate)} noValidate>
            <FieldGroup className="max-w-2xl gap-4">
              {errorMessage ? <FieldError>{errorMessage}</FieldError> : null}
              <Field data-invalid={Boolean(form.formState.errors.domain)}>
                <FieldLabel htmlFor="site-domain">Domain</FieldLabel>
                <Input
                  id="site-domain"
                  placeholder="site.example.com"
                  autoCapitalize="none"
                  autoCorrect="off"
                  spellCheck={false}
                  aria-invalid={Boolean(form.formState.errors.domain)}
                  {...form.register('domain')}
                />
                <FieldDescription>
                  Use an ASCII hostname you control. DNS automation arrives in Phase 3.
                </FieldDescription>
                <FieldError errors={[form.formState.errors.domain]} />
              </Field>
              <div>
                <Button
                  type="submit"
                  disabled={createMutation.isPending}
                  aria-busy={createMutation.isPending}
                >
                  {createMutation.isPending ? (
                    <Spinner data-icon="inline-start" />
                  ) : (
                    <Plus data-icon="inline-start" />
                  )}
                  {createMutation.isPending ? 'Queueing…' : 'Create site'}
                </Button>
              </div>
            </FieldGroup>
          </form>
        </CardContent>
      </Card>

      <section aria-labelledby="sites-heading" className="flex flex-col gap-4">
        <div className="flex items-end justify-between gap-4">
          <div>
            <p className="label-meta text-muted-foreground">Hosting</p>
            <h2 id="sites-heading" className="heading-section mt-1">
              Sites
            </h2>
          </div>
          <p className="text-sm text-muted-foreground tabular-nums">
            {sitesQuery.data.meta.total}{' '}
            {sitesQuery.data.meta.total === 1 ? 'site' : 'sites'}
          </p>
        </div>

        {sitesQuery.data.data.length === 0 ? (
          <Empty className="min-h-64 border">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <Server aria-hidden="true" />
              </EmptyMedia>
              <EmptyTitle>No sites yet</EmptyTitle>
              <EmptyDescription>
                Enter a domain above to queue your first isolated site.
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          <>
            <div className="hidden overflow-hidden rounded-lg border md:block">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Domain</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Created</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {sitesQuery.data.data.map((site) => (
                    <TableRow key={site.id}>
                      <TableCell>
                        <SiteIdentity site={site} />
                      </TableCell>
                      <TableCell>
                        <SiteStatusBadge site={site} />
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {formatDate(site.created_at)}
                      </TableCell>
                      <TableCell>
                        <SiteActions
                          site={site}
                          pending={
                            lifecycleMutation.isPending &&
                            lifecycleMutation.variables?.siteID === site.id
                          }
                          onAction={(action) =>
                            lifecycleMutation.mutateAsync({ siteID: site.id, action })
                          }
                        />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>

            <ul className="flex flex-col gap-3 md:hidden">
              {sitesQuery.data.data.map((site) => (
                <li key={site.id} className="flex flex-col gap-4 rounded-lg border p-4">
                  <div className="flex items-start justify-between gap-3">
                    <SiteIdentity site={site} />
                    <SiteStatusBadge site={site} />
                  </div>
                  <div className="flex items-center justify-between gap-3">
                    <p className="text-xs text-muted-foreground">
                      Created {formatDate(site.created_at)}
                    </p>
                    <SiteActions
                      site={site}
                      pending={
                        lifecycleMutation.isPending &&
                        lifecycleMutation.variables?.siteID === site.id
                      }
                      onAction={(action) =>
                        lifecycleMutation.mutateAsync({ siteID: site.id, action })
                      }
                    />
                  </div>
                </li>
              ))}
            </ul>
          </>
        )}
        {hasPendingSites(sitesQuery.data.data) ? (
          <p role="status" className="flex items-center gap-2 text-sm text-muted-foreground">
            <Spinner />
            Refreshing lifecycle status…
          </p>
        ) : null}
      </section>
    </div>
  );
}

function SiteIdentity({ site }: { site: Site }) {
  return (
    <div className="min-w-0">
      <p className="truncate font-medium">{site.domain}</p>
      <p className="mt-1 truncate font-mono text-xs text-muted-foreground">
        {site.id}
      </p>
      {site.status === 'failed' && site.last_error ? (
        <p className="mt-2 max-w-md text-xs text-destructive">
          Provisioning failed. Retry is exhausted; delete this site before trying again.
        </p>
      ) : null}
    </div>
  );
}

function SiteStatusBadge({ site }: { site: Site }) {
  const pending = ['provisioning', 'suspending', 'resuming', 'deleting'].includes(
    site.status,
  );
  const Icon = site.status === 'active'
    ? CircleCheck
    : site.status === 'failed'
      ? CircleAlert
      : Clock3;
  const variant = site.status === 'failed'
    ? 'destructive'
    : site.status === 'active'
      ? 'default'
      : 'secondary';
  return (
    <Badge variant={variant}>
      {pending ? <Spinner data-icon="inline-start" /> : <Icon data-icon="inline-start" />}
      {statusLabel[site.status]}
    </Badge>
  );
}

function SiteActions({
  site,
  pending,
  onAction,
}: {
  site: Site;
  pending: boolean;
  onAction: (action: LifecycleAction) => Promise<unknown>;
}) {
  return (
    <div className="flex justify-end gap-2">
      {site.status === 'active' ? (
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={pending}
          onClick={() => void onAction('suspend')}
        >
          <Pause data-icon="inline-start" />
          Suspend
        </Button>
      ) : null}
      {site.status === 'suspended' ? (
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={pending}
          onClick={() => void onAction('resume')}
        >
          <Play data-icon="inline-start" />
          Resume
        </Button>
      ) : null}
      {!['deleting', 'deleted'].includes(site.status) ? (
        <DeleteSiteDialog
          site={site}
          pending={pending}
          onDelete={() => onAction('delete')}
        />
      ) : null}
    </div>
  );
}

function DeleteSiteDialog({
  site,
  pending,
  onDelete,
}: {
  site: Site;
  pending: boolean;
  onDelete: () => Promise<unknown>;
}) {
  const [open, setOpen] = useState(false);
  const [confirmation, setConfirmation] = useState('');
  const matches = confirmation === site.domain;

  function handleOpenChange(next: boolean) {
    setOpen(next);
    if (!next) {
      setConfirmation('');
    }
  }

  async function handleDelete() {
    if (!matches) {
      return;
    }
    await onDelete();
    handleOpenChange(false);
  }

  return (
    <AlertDialog open={open} onOpenChange={handleOpenChange}>
      <AlertDialogTrigger render={<Button type="button" size="sm" variant="ghost" />}>
        <Trash2 data-icon="inline-start" />
        Delete
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogMedia>
            <Trash2 aria-hidden="true" />
          </AlertDialogMedia>
          <AlertDialogTitle>Delete {site.domain}?</AlertDialogTitle>
          <AlertDialogDescription>
            This removes the site container, route, network, and data volume. Type the
            domain to confirm.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <Field data-invalid={confirmation.length > 0 && !matches}>
          <FieldLabel htmlFor={`delete-${site.id}`}>Domain</FieldLabel>
          <Input
            id={`delete-${site.id}`}
            value={confirmation}
            onChange={(event) => setConfirmation(event.target.value)}
            aria-invalid={confirmation.length > 0 && !matches}
            autoComplete="off"
          />
        </Field>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>Cancel</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={!matches || pending}
            aria-busy={pending}
            onClick={() => void handleDelete()}
          >
            {pending ? <Spinner data-icon="inline-start" /> : <Trash2 data-icon="inline-start" />}
            {pending ? 'Queueing…' : 'Delete site'}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat('en', {
    dateStyle: 'medium',
  }).format(new Date(value));
}
