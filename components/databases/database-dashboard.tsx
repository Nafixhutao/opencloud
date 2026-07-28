'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  CircleAlert,
  CircleCheck,
  Clock3,
  Copy,
  Database,
  Eye,
  EyeOff,
  Plus,
  Trash2,
} from 'lucide-react';
import { useEffect, useState } from 'react';
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
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from '@/components/ui/field';
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
  createDatabase,
  DatabaseAPIError,
  type DatabaseCredentials,
  type DatabasesEnvelope,
  deleteDatabase,
  hasPendingDatabases,
  listDatabases,
  type ManagedDatabase,
  revealDatabaseCredentials,
} from '@/lib/databases';
import {
  createDatabaseSchema,
  type CreateDatabaseValues,
} from '@/lib/database-validation';

type DatabaseDashboardProps = {
  initialData: DatabasesEnvelope;
};

const statusLabel: Record<ManagedDatabase['status'], string> = {
  provisioning: 'Provisioning',
  active: 'Active',
  deleting: 'Deleting',
  deleted: 'Deleted',
  failed: 'Failed',
};

export function DatabaseDashboard({ initialData }: DatabaseDashboardProps) {
  const queryClient = useQueryClient();
  const perPage = initialData.meta.per_page || 25;
  const [page, setPage] = useState(initialData.meta.page || 1);
  const [revealed, setRevealed] = useState<{
    database: ManagedDatabase;
    credentials: DatabaseCredentials;
  } | null>(null);
  const form = useForm<CreateDatabaseValues>({
    resolver: zodResolver(createDatabaseSchema),
    defaultValues: { name: '', engine: 'postgres' },
  });
  const databasesQuery = useQuery({
    queryKey: ['databases', page, perPage],
    queryFn: () => listDatabases({ page, perPage }),
    initialData: page === initialData.meta.page ? initialData : undefined,
    refetchInterval: (query) =>
      query.state.data && hasPendingDatabases(query.state.data.data) ? 2_000 : false,
  });

  const createMutation = useMutation({
    mutationFn: ({
      name,
      engine,
      key,
    }: CreateDatabaseValues & { key: string }) => createDatabase(name, engine, key),
    onSuccess: async () => {
      form.reset({ name: '', engine: 'postgres' });
      setPage(1);
      await queryClient.invalidateQueries({ queryKey: ['databases'] });
    },
  });
  const deleteMutation = useMutation({
    mutationFn: deleteDatabase,
    onSuccess: async (_response, databaseID) => {
      if (revealed?.database.id === databaseID) {
        setRevealed(null);
      }
      await queryClient.invalidateQueries({ queryKey: ['databases'] });
    },
  });
  const databasesEnvelope = databasesQuery.data ?? {
    data: [],
    meta: {
      page,
      per_page: perPage,
      total: initialData.meta.total,
    },
  };
  const totalPages = Math.max(
    1,
    Math.ceil(databasesEnvelope.meta.total / databasesEnvelope.meta.per_page),
  );

  useEffect(() => {
    if (
      databasesQuery.isFetching ||
      !databasesQuery.data ||
      databasesQuery.data.data.length > 0 ||
      page <= 1
    ) {
      return;
    }
    const lastPage = Math.max(
      1,
      Math.ceil(
        databasesQuery.data.meta.total / databasesQuery.data.meta.per_page,
      ),
    );
    if (page > lastPage) {
      setPage(lastPage);
    }
  }, [databasesQuery.data, databasesQuery.isFetching, page]);
  const revealMutation = useMutation({
    mutationFn: ({
      database,
    }: {
      database: ManagedDatabase;
    }) => revealDatabaseCredentials(database.id),
    onSuccess: async (response, variables) => {
      setRevealed({ database: variables.database, credentials: response.data });
      await queryClient.invalidateQueries({ queryKey: ['databases'] });
    },
  });

  async function onCreate(values: CreateDatabaseValues) {
    try {
      await createMutation.mutateAsync({ ...values, key: crypto.randomUUID() });
    } catch {
      // TanStack Query owns the displayed mutation error.
    }
  }

  const mutationError =
    createMutation.error ?? deleteMutation.error ?? revealMutation.error;
  const errorMessage =
    mutationError instanceof DatabaseAPIError
      ? mutationError.message
      : mutationError
        ? 'The control plane could not complete the request.'
        : null;

  return (
    <div className="flex flex-col gap-10">
      <Card>
        <CardHeader>
          <CardTitle>Create a database</CardTitle>
          <CardDescription>
            Choose an engine and a customer-facing label. A worker creates an isolated
            physical database and scoped login on the configured data plane.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={form.handleSubmit(onCreate)} noValidate>
            <FieldGroup className="max-w-2xl gap-4">
              {errorMessage ? <FieldError>{errorMessage}</FieldError> : null}
              <Field data-invalid={Boolean(form.formState.errors.name)}>
                <FieldLabel htmlFor="database-name">Name</FieldLabel>
                <Input
                  id="database-name"
                  placeholder="orders"
                  autoCapitalize="none"
                  autoCorrect="off"
                  spellCheck={false}
                  aria-invalid={Boolean(form.formState.errors.name)}
                  {...form.register('name')}
                />
                <FieldDescription>
                  Start with a letter. Use lowercase letters, numbers, underscores, or
                  hyphens.
                </FieldDescription>
                <FieldError errors={[form.formState.errors.name]} />
              </Field>
              <Field data-invalid={Boolean(form.formState.errors.engine)}>
                <FieldLabel htmlFor="database-engine">Engine</FieldLabel>
                <select
                  id="database-engine"
                  className="h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-xs outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
                  aria-invalid={Boolean(form.formState.errors.engine)}
                  {...form.register('engine')}
                >
                  <option value="postgres">PostgreSQL</option>
                  <option value="mariadb">MariaDB</option>
                </select>
                <FieldDescription>
                  Both engines receive a dedicated internal database and login.
                </FieldDescription>
                <FieldError errors={[form.formState.errors.engine]} />
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
                  {createMutation.isPending ? 'Queueing…' : 'Create database'}
                </Button>
              </div>
            </FieldGroup>
          </form>
        </CardContent>
      </Card>

      {revealed ? (
        <CredentialPanel
          database={revealed.database}
          credentials={revealed.credentials}
          onHide={() => setRevealed(null)}
        />
      ) : null}

      <section aria-labelledby="databases-heading" className="flex flex-col gap-4">
        <div className="flex items-end justify-between gap-4">
          <div>
            <p className="label-meta text-muted-foreground">Data</p>
            <h2 id="databases-heading" className="heading-section mt-1">
              Databases
            </h2>
          </div>
          <p className="text-sm text-muted-foreground tabular-nums">
            {databasesEnvelope.meta.total}{' '}
            {databasesEnvelope.meta.total === 1 ? 'database' : 'databases'}
          </p>
        </div>

        {databasesQuery.isPending ? (
          <div
            role="status"
            className="flex min-h-64 items-center justify-center gap-2 rounded-lg border text-sm text-muted-foreground"
          >
            <Spinner />
            Loading databasesâ€¦
          </div>
        ) : databasesEnvelope.data.length === 0 ? (
          <Empty className="min-h-64 border">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <Database aria-hidden="true" />
              </EmptyMedia>
              <EmptyTitle>No databases yet</EmptyTitle>
              <EmptyDescription>
                Create a PostgreSQL or MariaDB database above. Provisioning runs in the
                background.
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          <>
            <div className="hidden overflow-hidden rounded-lg border md:block">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Engine</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Created</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {databasesEnvelope.data.map((database) => (
                    <TableRow key={database.id}>
                      <TableCell>
                        <DatabaseIdentity database={database} />
                      </TableCell>
                      <TableCell>{engineLabel(database.engine)}</TableCell>
                      <TableCell>
                        <DatabaseStatusBadge database={database} />
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {formatDate(database.created_at)}
                      </TableCell>
                      <TableCell>
                        <DatabaseActions
                          database={database}
                          actionPending={
                            (deleteMutation.isPending &&
                              deleteMutation.variables === database.id) ||
                            (revealMutation.isPending &&
                              revealMutation.variables?.database.id === database.id)
                          }
                          revealDisabled={revealed !== null}
                          onReveal={() => revealMutation.mutateAsync({ database })}
                          onDelete={() => deleteMutation.mutateAsync(database.id)}
                        />
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>

            <ul className="flex flex-col gap-3 md:hidden">
              {databasesEnvelope.data.map((database) => (
                <li key={database.id} className="flex flex-col gap-4 rounded-lg border p-4">
                  <div className="flex items-start justify-between gap-3">
                    <DatabaseIdentity database={database} />
                    <DatabaseStatusBadge database={database} />
                  </div>
                  <p className="text-sm text-muted-foreground">
                    {engineLabel(database.engine)} · Created {formatDate(database.created_at)}
                  </p>
                  <DatabaseActions
                    database={database}
                    actionPending={
                      (deleteMutation.isPending &&
                        deleteMutation.variables === database.id) ||
                      (revealMutation.isPending &&
                        revealMutation.variables?.database.id === database.id)
                    }
                    revealDisabled={revealed !== null}
                    onReveal={() => revealMutation.mutateAsync({ database })}
                    onDelete={() => deleteMutation.mutateAsync(database.id)}
                  />
                </li>
              ))}
            </ul>
          </>
        )}
        {totalPages > 1 ? (
          <nav
            aria-label="Database pagination"
            className="flex flex-wrap items-center justify-between gap-3 border-t pt-4"
          >
            <p className="text-sm text-muted-foreground tabular-nums">
              Page {page} of {totalPages}
            </p>
            <div className="flex gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={page <= 1 || databasesQuery.isFetching}
                onClick={() => setPage((current) => Math.max(1, current - 1))}
              >
                Previous
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={page >= totalPages || databasesQuery.isFetching}
                onClick={() => setPage((current) => Math.min(totalPages, current + 1))}
              >
                Next
              </Button>
            </div>
          </nav>
        ) : null}
        {hasPendingDatabases(databasesEnvelope.data) ? (
          <p role="status" className="flex items-center gap-2 text-sm text-muted-foreground">
            <Spinner />
            Refreshing lifecycle status…
          </p>
        ) : null}
      </section>
    </div>
  );
}

function DatabaseIdentity({ database }: { database: ManagedDatabase }) {
  return (
    <div className="min-w-0">
      <p className="truncate font-medium">{database.name}</p>
      <p className="mt-1 truncate font-mono text-xs text-muted-foreground">
        {database.id}
      </p>
      {database.status === 'failed' && database.last_error ? (
        <p className="mt-2 max-w-md text-xs text-destructive">
          Provisioning failed after all retries. Delete this resource before trying again.
        </p>
      ) : null}
    </div>
  );
}

function DatabaseStatusBadge({ database }: { database: ManagedDatabase }) {
  const pending = ['provisioning', 'deleting'].includes(database.status);
  const Icon =
    database.status === 'active'
      ? CircleCheck
      : database.status === 'failed'
        ? CircleAlert
        : Clock3;
  const variant =
    database.status === 'failed'
      ? 'destructive'
      : database.status === 'active'
        ? 'default'
        : 'secondary';
  return (
    <Badge variant={variant}>
      {pending ? <Spinner data-icon="inline-start" /> : <Icon data-icon="inline-start" />}
      {statusLabel[database.status]}
    </Badge>
  );
}

function DatabaseActions({
  database,
  actionPending,
  revealDisabled,
  onReveal,
  onDelete,
}: {
  database: ManagedDatabase;
  actionPending: boolean;
  revealDisabled: boolean;
  onReveal: () => Promise<unknown>;
  onDelete: () => Promise<unknown>;
}) {
  return (
    <div className="flex justify-end gap-2">
      {database.status === 'active' && database.credential_available ? (
        <RevealCredentialDialog
          database={database}
          pending={actionPending}
          disabled={revealDisabled}
          onReveal={onReveal}
        />
      ) : null}
      {!['deleting', 'deleted'].includes(database.status) ? (
        <DeleteDatabaseDialog
          database={database}
          pending={actionPending}
          onDelete={onDelete}
        />
      ) : null}
    </div>
  );
}

function RevealCredentialDialog({
  database,
  pending,
  disabled,
  onReveal,
}: {
  database: ManagedDatabase;
  pending: boolean;
  disabled: boolean;
  onReveal: () => Promise<unknown>;
}) {
  const [open, setOpen] = useState(false);

  async function handleReveal() {
    try {
      await onReveal();
      setOpen(false);
    } catch {
      // Keep the confirmation open; the dashboard renders the mutation error.
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={setOpen}>
      <AlertDialogTrigger
        render={
          <Button type="button" size="sm" variant="outline" disabled={disabled || pending} />
        }
      >
        <Eye data-icon="inline-start" />
        Reveal once
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogMedia>
            <Eye aria-hidden="true" />
          </AlertDialogMedia>
          <AlertDialogTitle>Reveal credentials for {database.name}?</AlertDialogTitle>
          <AlertDialogDescription>
            Save them immediately. After this response, the encrypted copy is permanently
            removed and cannot be revealed again.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>Cancel</AlertDialogCancel>
          <AlertDialogAction
            disabled={pending}
            aria-busy={pending}
            onClick={() => void handleReveal()}
          >
            {pending ? <Spinner data-icon="inline-start" /> : <Eye data-icon="inline-start" />}
            {pending ? 'Revealing…' : 'Reveal credentials'}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function DeleteDatabaseDialog({
  database,
  pending,
  onDelete,
}: {
  database: ManagedDatabase;
  pending: boolean;
  onDelete: () => Promise<unknown>;
}) {
  const [open, setOpen] = useState(false);
  const [confirmation, setConfirmation] = useState('');
  const matches = confirmation === database.name;

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
    try {
      await onDelete();
      handleOpenChange(false);
    } catch {
      // Keep the confirmation open; the dashboard renders the mutation error.
    }
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
          <AlertDialogTitle>Delete {database.name}?</AlertDialogTitle>
          <AlertDialogDescription>
            This permanently removes the physical database and scoped user. Any
            unrevealed credential is revoked. Type the database name to confirm.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <Field data-invalid={confirmation.length > 0 && !matches}>
          <FieldLabel htmlFor={`delete-database-${database.id}`}>Database name</FieldLabel>
          <Input
            id={`delete-database-${database.id}`}
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
            {pending ? 'Queueing…' : 'Delete database'}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function CredentialPanel({
  database,
  credentials,
  onHide,
}: {
  database: ManagedDatabase;
  credentials: DatabaseCredentials;
  onHide: () => void;
}) {
  const [copied, setCopied] = useState(false);
  const [copyFailed, setCopyFailed] = useState(false);
  const lines = [
    `ENGINE=${credentials.engine}`,
    `HOST=${credentials.host}`,
    `PORT=${credentials.port}`,
    `DATABASE=${credentials.database}`,
    `USERNAME=${credentials.username}`,
    `PASSWORD=${credentials.password}`,
    `TLS_REQUIRED=${credentials.tls_required}`,
  ].join('\n');

  async function copyCredentials() {
    try {
      await navigator.clipboard.writeText(lines);
      setCopied(true);
      setCopyFailed(false);
    } catch {
      setCopied(false);
      setCopyFailed(true);
    }
  }

  return (
    <Card aria-live="polite">
      <CardHeader>
        <CardTitle>Save credentials for {database.name}</CardTitle>
        <CardDescription>
          This is the only display. Store these values in a password manager or secret
          manager before hiding them.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-5">
        <dl className="grid gap-x-6 gap-y-4 sm:grid-cols-2">
          <CredentialValue label="Engine" value={engineLabel(credentials.engine)} />
          <CredentialValue label="TLS" value={credentials.tls_required ? 'Required' : 'Not required'} />
          <CredentialValue label="Host" value={credentials.host} />
          <CredentialValue label="Port" value={String(credentials.port)} />
          <CredentialValue label="Database" value={credentials.database} />
          <CredentialValue label="Username" value={credentials.username} />
          <div className="sm:col-span-2">
            <CredentialValue label="Password" value={credentials.password} />
          </div>
        </dl>
        {copyFailed ? (
          <p role="alert" className="text-sm text-destructive">
            Clipboard access failed. Copy each value manually before leaving this page.
          </p>
        ) : null}
        {copied ? (
          <p role="status" className="text-sm text-muted-foreground">
            Credentials copied. Move them into secure storage now.
          </p>
        ) : null}
        <div className="flex flex-wrap gap-2">
          <Button type="button" onClick={() => void copyCredentials()}>
            <Copy data-icon="inline-start" />
            Copy all
          </Button>
          <HideCredentialDialog onHide={onHide} />
        </div>
      </CardContent>
    </Card>
  );
}

function CredentialValue({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs font-medium text-muted-foreground">{label}</dt>
      <dd className="mt-1 break-all font-mono text-sm">{value}</dd>
    </div>
  );
}

function HideCredentialDialog({ onHide }: { onHide: () => void }) {
  return (
    <AlertDialog>
      <AlertDialogTrigger render={<Button type="button" variant="outline" />}>
        <EyeOff data-icon="inline-start" />
        Hide credentials
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogMedia>
            <EyeOff aria-hidden="true" />
          </AlertDialogMedia>
          <AlertDialogTitle>Have you saved these credentials?</AlertDialogTitle>
          <AlertDialogDescription>
            Hiding removes them from this browser view. The control plane has already
            deleted its encrypted copy, so they cannot be shown again.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Keep visible</AlertDialogCancel>
          <AlertDialogAction onClick={onHide}>I saved them</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function engineLabel(engine: ManagedDatabase['engine']): string {
  return engine === 'postgres' ? 'PostgreSQL' : 'MariaDB';
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat('en', {
    dateStyle: 'medium',
  }).format(new Date(value));
}
