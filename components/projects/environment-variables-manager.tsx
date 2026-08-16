'use client';

import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  CheckIcon,
  CopyIcon,
  EyeIcon,
  EyeOffIcon,
  HistoryIcon,
  PlusIcon,
  VariableIcon,
} from 'lucide-react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';

import { Button } from '@/components/ui/button';
import { Card, CardHeader, CardTitle, CardAction, CardContent } from '@/components/ui/card';
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Spinner } from '@/components/ui/spinner';
import { Skeleton } from '@/components/ui/skeleton';
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty';
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogHeader,
  AlertDialogFooter,
  AlertDialogMedia,
  AlertDialogTitle,
  AlertDialogDescription,
  AlertDialogAction,
  AlertDialogCancel,
} from '@/components/ui/alert-dialog';
import { FieldGroup, Field, FieldLabel, FieldError, FieldDescription } from '@/components/ui/field';

import {
  listEnvironmentVariables,
  listEnvironmentVariableAudit,
  createEnvironmentVariable,
  updateEnvironmentVariable,
  deleteEnvironmentVariable,
  revealSecret,
  type EnvironmentVariable,
  type EnvironmentVariableEnvironment,
} from '@/lib/environment-variables';
import {
  createEnvironmentVariableSchema,
  updateEnvironmentVariableSchema,
  ENVIRONMENT_VARIABLE_ENVIRONMENTS,
  type CreateEnvironmentVariableValues,
} from '@/lib/environment-variable-validation';

export type EnvironmentVariablesService = { id: string; name: string };

type Props = { projectId: string; services: EnvironmentVariablesService[] };

const ENVIRONMENT_LABELS: Record<EnvironmentVariableEnvironment, string> = {
  production: 'Production',
  preview: 'Preview',
  development: 'Development',
};

const AUDIT_ACTION_BADGES: Record<string, 'default' | 'secondary' | 'destructive'> = {
  created: 'default',
  updated: 'secondary',
  rotated: 'secondary',
  revealed: 'secondary',
  deleted: 'destructive',
};

export function EnvironmentVariablesManager({ projectId, services }: Props) {
  const queryClient = useQueryClient();
  const [serviceId, setServiceId] = useState(services[0]?.id ?? '');
  const [environment, setEnvironment] = useState<EnvironmentVariableEnvironment>('production');
  const [showCreateForm, setShowCreateForm] = useState(false);
  const [editingVariable, setEditingVariable] = useState<EnvironmentVariable | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<EnvironmentVariable | null>(null);
  const [deleteConfirmKey, setDeleteConfirmKey] = useState('');
  const [revealed, setRevealed] = useState<Record<string, string>>({});
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [showAudit, setShowAudit] = useState(false);

  const hasServices = services.length > 0 && serviceId !== '';

  const { data: variables, isLoading, isError, refetch } = useQuery({
    queryKey: ['environment-variables', projectId, serviceId, environment],
    queryFn: () => listEnvironmentVariables(projectId, serviceId, environment),
    enabled: hasServices,
    staleTime: 10_000,
  });

  const { data: auditEntries, isLoading: auditLoading, isError: auditError, refetch: refetchAudit } = useQuery({
    queryKey: ['environment-variable-audit', projectId, serviceId],
    queryFn: () => listEnvironmentVariableAudit(projectId, serviceId, 20),
    enabled: hasServices && showAudit,
    staleTime: 10_000,
  });

  const invalidateList = () => {
    void queryClient.invalidateQueries({ queryKey: ['environment-variables', projectId, serviceId] });
    void queryClient.invalidateQueries({ queryKey: ['environment-variable-audit', projectId, serviceId] });
  };

  const createForm = useForm<CreateEnvironmentVariableValues>({
    resolver: zodResolver(createEnvironmentVariableSchema),
    defaultValues: { key: '', value: '', is_secret: false, environment: 'production' },
  });

  const createMutation = useMutation({
    mutationFn: (values: CreateEnvironmentVariableValues) =>
      createEnvironmentVariable(projectId, serviceId, values),
    onSuccess: () => {
      setShowCreateForm(false);
      createForm.reset({ key: '', value: '', is_secret: false, environment });
      void invalidateList();
    },
  });

  const editForm = useForm<{ value: string }>({
    resolver: zodResolver(updateEnvironmentVariableSchema),
    defaultValues: { value: '' },
  });

  const updateMutation = useMutation({
    mutationFn: ({ id, value }: { id: string; value: string }) =>
      updateEnvironmentVariable(projectId, serviceId, id, { value }),
    onSuccess: (_data, variable) => {
      setEditingVariable(null);
      setRevealed((current) => {
        if (!variable) return current;
        const next = { ...current };
        delete next[variable.id];
        return next;
      });
      void invalidateList();
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteEnvironmentVariable(projectId, serviceId, id),
    onSuccess: () => {
      setDeleteTarget(null);
      setDeleteConfirmKey('');
      void invalidateList();
    },
  });

  const revealMutation = useMutation({
    mutationFn: (id: string) => revealSecret(projectId, serviceId, id),
    onSuccess: (value, id) => setRevealed((current) => ({ ...current, [id]: value })),
  });

  const handleCopy = async (text: string, id: string) => {
    await navigator.clipboard.writeText(text);
    setCopiedId(id);
    setTimeout(() => setCopiedId((current) => (current === id ? null : current)), 2000);
  };

  const list = variables ?? [];

  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>Environment Variables</CardTitle>
        <CardAction>
          <Button
            size="sm"
            disabled={!hasServices}
            onClick={() => {
              setShowCreateForm(!showCreateForm);
              createForm.reset({ key: '', value: '', is_secret: false, environment });
            }}
          >
            <PlusIcon data-icon="inline-start" />
            Add Variable
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent>
        <div className="mb-4 flex flex-wrap gap-3">
          {services.length > 1 && (
            <label>
              <span className="sr-only">Service</span>
              <select
                aria-label="Service"
                value={serviceId}
                onChange={(event) => {
                  setServiceId(event.target.value);
                  setRevealed({});
                }}
                className="h-9 rounded-sm border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                {services.map((service) => (
                  <option key={service.id} value={service.id}>{service.name}</option>
                ))}
              </select>
            </label>
          )}
          <label>
            <span className="sr-only">Environment</span>
            <select
              aria-label="Environment"
              value={environment}
              onChange={(event) => {
                setEnvironment(event.target.value as EnvironmentVariableEnvironment);
                setRevealed({});
              }}
              className="h-9 rounded-sm border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              {ENVIRONMENT_VARIABLE_ENVIRONMENTS.map((value) => (
                <option key={value} value={value}>{ENVIRONMENT_LABELS[value]}</option>
              ))}
            </select>
          </label>
        </div>

        {!hasServices ? (
          <Empty className="min-h-48 border">
            <EmptyHeader>
              <EmptyMedia variant="icon"><VariableIcon aria-hidden="true" /></EmptyMedia>
              <EmptyTitle>No service to configure</EmptyTitle>
              <EmptyDescription>Add a service to this project before managing its environment.</EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : showCreateForm ? (
          <form
            className="mb-6 rounded-lg border p-4"
            onSubmit={createForm.handleSubmit((values) => createMutation.mutate(values))}
            noValidate
          >
            <FieldGroup>
              <Field data-invalid={!!createForm.formState.errors.key}>
                <FieldLabel htmlFor="env-var-key">Key</FieldLabel>
                <Input
                  id="env-var-key"
                  placeholder="DATABASE_URL"
                  autoComplete="off"
                  {...createForm.register('key')}
                  aria-describedby={createForm.formState.errors.key ? 'env-key-error' : undefined}
                />
                <FieldError id="env-key-error">{createForm.formState.errors.key?.message}</FieldError>
              </Field>
              <Field data-invalid={!!createForm.formState.errors.value}>
                <FieldLabel htmlFor="env-var-value">Value</FieldLabel>
                <Input
                  id="env-var-value"
                  placeholder="postgresql://…"
                  autoComplete="off"
                  {...createForm.register('value')}
                  aria-describedby={createForm.formState.errors.value ? 'env-value-error' : undefined}
                />
                <FieldError id="env-value-error">{createForm.formState.errors.value?.message}</FieldError>
              </Field>
              <Field>
                <FieldLabel>Scope</FieldLabel>
                <div className="flex flex-wrap items-center gap-4">
                  <label className="inline-flex items-center gap-2 text-sm">
                    <input
                      type="checkbox"
                      checked={createForm.watch('is_secret')}
                      onChange={(event) => createForm.setValue('is_secret', event.target.checked)}
                    />
                    Secret (encrypted, never shown in lists)
                  </label>
                  <label className="inline-flex items-center gap-2 text-sm">
                    <span className="sr-only">Environment</span>
                    <select
                      aria-label="Environment"
                      {...createForm.register('environment')}
                      className="h-9 rounded-sm border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    >
                      {ENVIRONMENT_VARIABLE_ENVIRONMENTS.map((value) => (
                        <option key={value} value={value}>{ENVIRONMENT_LABELS[value]}</option>
                      ))}
                    </select>
                  </label>
                </div>
                <FieldDescription>Secrets are encrypted at rest and only readable through an audited reveal.</FieldDescription>
              </Field>
            </FieldGroup>
            {createMutation.error && (
              <p role="alert" className="mb-3 mt-1 text-sm text-destructive">
                {createMutation.error instanceof Error ? createMutation.error.message : 'Failed to create the variable.'}
              </p>
            )}
            <div className="flex gap-2">
              <Button type="submit" size="sm" disabled={createMutation.isPending}>
                {createMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
                {createMutation.isPending ? 'Saving…' : 'Save Variable'}
              </Button>
              <Button type="button" variant="outline" size="sm" onClick={() => setShowCreateForm(false)}>
                Cancel
              </Button>
            </div>
          </form>
        ) : isLoading ? (
          <div aria-label="Loading environment variables" className="flex flex-col gap-2">
            {[...Array(3)].map((_, index) => <Skeleton key={index} className="h-10 w-full" />)}
          </div>
        ) : isError ? (
          <div className="py-8 text-center">
            <p className="text-sm text-destructive">Could not load environment variables.</p>
            <Button variant="outline" size="sm" className="mt-2" onClick={() => void refetch()}>Retry</Button>
          </div>
        ) : list.length === 0 ? (
          <Empty className="min-h-48 border">
            <EmptyHeader>
              <EmptyMedia variant="icon"><VariableIcon aria-hidden="true" /></EmptyMedia>
              <EmptyTitle>No variables in {ENVIRONMENT_LABELS[environment].toLowerCase()}</EmptyTitle>
              <EmptyDescription>Add the first variable or secret for this environment.</EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          <div className="overflow-hidden rounded-lg border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Key</TableHead>
                  <TableHead>Type</TableHead>
                  <TableHead>Value</TableHead>
                  <TableHead>Updated</TableHead>
                  <TableHead className="w-0" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {list.map((variable) => (
                  <TableRow key={variable.id}>
                    <TableCell className="font-mono font-medium">{variable.key}</TableCell>
                    <TableCell>
                      <Badge variant={variable.is_secret ? 'default' : 'secondary'}>
                        {variable.is_secret ? 'Secret' : 'Variable'}
                      </Badge>
                    </TableCell>
                    <TableCell className="max-w-72">
                      {variable.is_secret ? (
                        revealed[variable.id] ? (
                          <div className="flex items-center gap-1">
                            <code className="truncate font-mono text-xs">{revealed[variable.id]}</code>
                            <Button
                              variant="ghost"
                              size="icon-xs"
                              aria-label={`Copy revealed value of ${variable.key}`}
                              onClick={() => void handleCopy(revealed[variable.id], variable.id)}
                            >
                              {copiedId === variable.id ? <CheckIcon aria-hidden="true" /> : <CopyIcon aria-hidden="true" />}
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon-xs"
                              aria-label={`Hide value of ${variable.key}`}
                              onClick={() => setRevealed((current) => {
                                const next = { ...current };
                                delete next[variable.id];
                                return next;
                              })}
                            >
                              <EyeOffIcon aria-hidden="true" />
                            </Button>
                          </div>
                        ) : (
                          <Button
                            variant="outline"
                            size="xs"
                            aria-label={`Reveal value of ${variable.key}`}
                            disabled={revealMutation.isPending && revealMutation.variables === variable.id}
                            onClick={() => revealMutation.mutate(variable.id)}
                          >
                            <EyeIcon data-icon="inline-start" />
                            Reveal
                          </Button>
                        )
                      ) : (
                        <div className="flex items-center gap-1">
                          <code className="truncate font-mono text-xs">{variable.value}</code>
                          <Button
                            variant="ghost"
                            size="icon-xs"
                            aria-label={`Copy value of ${variable.key}`}
                            onClick={() => void handleCopy(variable.value ?? '', variable.id)}
                          >
                            {copiedId === variable.id ? <CheckIcon aria-hidden="true" /> : <CopyIcon aria-hidden="true" />}
                          </Button>
                        </div>
                      )}
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {new Date(variable.updated_at).toLocaleDateString()}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-1">
                        <Button
                          variant="outline"
                          size="xs"
                          onClick={() => {
                            setEditingVariable(variable);
                            editForm.reset({ value: '' });
                          }}
                        >
                          Edit
                        </Button>
                        <Button
                          variant="destructive"
                          size="xs"
                          onClick={() => {
                            setDeleteTarget(variable);
                            setDeleteConfirmKey('');
                          }}
                        >
                          Delete
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
        {hasServices && (
          <div className="mt-6 rounded-lg border">
            <div className="flex items-center justify-between px-4 py-3">
              <p className="flex items-center gap-2 text-sm font-medium">
                <HistoryIcon size={14} aria-hidden="true" />
                Recent activity
              </p>
              <Button variant="ghost" size="xs" aria-expanded={showAudit} onClick={() => setShowAudit((open) => !open)}>
                {showAudit ? 'Hide' : 'Show'}
              </Button>
            </div>
            {showAudit && (
              <div className="border-t px-4 py-3">
                {auditLoading ? (
                  <div aria-label="Loading environment activity" className="flex flex-col gap-2 py-1">
                    {[...Array(3)].map((_, index) => <Skeleton key={index} className="h-6 w-full" />)}
                  </div>
                ) : auditError ? (
                  <div className="py-2 text-center">
                    <p className="text-sm text-destructive">Could not load the activity trail.</p>
                    <Button variant="outline" size="xs" className="mt-2" onClick={() => void refetchAudit()}>Retry</Button>
                  </div>
                ) : (auditEntries?.length ?? 0) === 0 ? (
                  <p className="py-2 text-sm text-muted-foreground">
                    No recorded variable activity for this service yet.
                  </p>
                ) : (
                  <ul className="flex flex-col gap-2">
                    {(auditEntries ?? []).map((entry) => (
                      <li key={entry.id} className="flex flex-wrap items-center gap-2 text-sm">
                        <Badge variant={AUDIT_ACTION_BADGES[entry.action] ?? 'secondary'}>{entry.action}</Badge>
                        <code className="font-mono text-xs">{entry.key}</code>
                        {entry.is_secret && <Badge variant="outline">secret</Badge>}
                        <span className="text-xs text-muted-foreground">
                          {ENVIRONMENT_LABELS[entry.environment as EnvironmentVariableEnvironment] ?? entry.environment}
                        </span>
                        <span className="ml-auto text-xs text-muted-foreground">
                          {new Date(entry.created_at).toLocaleString()}
                        </span>
                      </li>
                    ))}
                  </ul>
                )}
                <p className="mt-3 text-xs text-muted-foreground">
                  Recorded server-side for every create, update, delete, and reveal — values are never included.
                </p>
              </div>
            )}
          </div>
        )}
        {revealMutation.error && (
          <p role="alert" className="mt-3 text-sm text-destructive">
            {revealMutation.error instanceof Error ? revealMutation.error.message : 'Failed to reveal the secret.'}
          </p>
        )}
      </CardContent>

      <AlertDialog
        open={!!editingVariable}
        onOpenChange={(open) => {
          if (!open) setEditingVariable(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia />
            <AlertDialogTitle>Update {editingVariable?.key}</AlertDialogTitle>
            <AlertDialogDescription>
              {editingVariable?.is_secret
                ? 'Enter the new secret value. The previous value is replaced and the change is audited.'
                : 'Enter the new value for this variable.'}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <form
            id="env-edit-form"
            onSubmit={editForm.handleSubmit((values) => {
              if (editingVariable) updateMutation.mutate({ id: editingVariable.id, value: values.value });
            })}
            noValidate
          >
            <Field data-invalid={!!editForm.formState.errors.value}>
              <FieldLabel htmlFor="env-edit-value">New value</FieldLabel>
              <Input
                id="env-edit-value"
                type={editingVariable?.is_secret ? 'password' : 'text'}
                autoComplete="off"
                {...editForm.register('value')}
                aria-describedby={editForm.formState.errors.value ? 'env-edit-error' : undefined}
              />
              <FieldError id="env-edit-error">{editForm.formState.errors.value?.message}</FieldError>
            </Field>
          </form>
          {updateMutation.error && (
            <p role="alert" className="text-sm text-destructive">
              {updateMutation.error instanceof Error ? updateMutation.error.message : 'Failed to update the variable.'}
            </p>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel onClick={() => setEditingVariable(null)}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              type="submit"
              form="env-edit-form"
              disabled={updateMutation.isPending}
            >
              {updateMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
              Save Value
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog
        open={!!deleteTarget}
        onOpenChange={(open) => {
          if (!open) {
            setDeleteTarget(null);
            setDeleteConfirmKey('');
          }
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia />
            <AlertDialogTitle>Delete Variable</AlertDialogTitle>
            <AlertDialogDescription>
              This action cannot be undone. Type <strong>{deleteTarget?.key}</strong> to confirm.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <Input
            value={deleteConfirmKey}
            onChange={(event) => setDeleteConfirmKey(event.target.value)}
            placeholder={deleteTarget?.key}
            aria-label="Type the variable key to confirm deletion"
          />
          {deleteMutation.error && (
            <p role="alert" className="text-sm text-destructive">
              {deleteMutation.error instanceof Error ? deleteMutation.error.message : 'Deletion failed.'}
            </p>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel onClick={() => { setDeleteTarget(null); setDeleteConfirmKey(''); }}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={deleteConfirmKey !== deleteTarget?.key || deleteMutation.isPending}
              onClick={() => { if (deleteTarget) deleteMutation.mutate(deleteTarget.id); }}
            >
              {deleteMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
              Delete Variable
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}
