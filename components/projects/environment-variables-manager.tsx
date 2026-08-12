'use client';

import { useState, useEffect, useCallback, useRef } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
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
} from '@/components/ui/alert-dialog';
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Spinner } from '@/components/ui/spinner';
import { Skeleton } from '@/components/ui/skeleton';
import { Badge } from '@/components/ui/badge';
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty';
import {
  listEnvironmentVariables,
  createEnvironmentVariable,
  updateEnvironmentVariable,
  deleteEnvironmentVariable,
  revealSecret,
  type EnvironmentVariable,
} from '@/lib/environment-variables';
import { Eye, EyeOff, Plus, Trash2, Edit2, Copy, Check, Variable, Trash2 as TrashIcon } from 'lucide-react';

type Environment = 'production' | 'preview' | 'development';

interface EnvironmentVariablesManagerProps {
  projectId: string;
  serviceId: string;
}

export function EnvironmentVariablesManager({
  projectId,
  serviceId,
}: EnvironmentVariablesManagerProps) {
  const [environment, setEnvironment] = useState<Environment>('production');
  const [variables, setVariables] = useState<EnvironmentVariable[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [showCreateForm, setShowCreateForm] = useState(false);
  const [editingVariable, setEditingVariable] = useState<EnvironmentVariable | null>(null);
  const [deletingVariable, setDeletingVariable] = useState<EnvironmentVariable | null>(null);
  const [revealedSecrets, setRevealedSecrets] = useState<Record<string, string>>({});
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [deletePending, setDeletePending] = useState(false);
  const copyTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (copyTimerRef.current) clearTimeout(copyTimerRef.current);
    };
  }, []);

  const loadVariables = useCallback(async () => {
    try {
      setLoading(true);
      setLoadError(null);
      const data = await listEnvironmentVariables(projectId, serviceId, environment);
      setVariables(data);
    } catch {
      setLoadError('Failed to load environment variables. Please try again.');
    } finally {
      setLoading(false);
    }
  }, [projectId, serviceId, environment]);

  useEffect(() => {
    void loadVariables();
  }, [loadVariables]);

  const handleRevealSecret = async (id: string) => {
    try {
      setActionError(null);
      const value = await revealSecret(projectId, serviceId, id);
      setRevealedSecrets((prev) => ({ ...prev, [id]: value }));
    } catch {
      setActionError('Failed to reveal secret. Please try again.');
    }
  };

  const handleHideSecret = (id: string) => {
    setRevealedSecrets((prev) => {
      const next = { ...prev };
      delete next[id];
      return next;
    });
  };

  const handleCopy = async (text: string, id: string) => {
    await navigator.clipboard.writeText(text);
    setCopiedId(id);
    if (copyTimerRef.current) clearTimeout(copyTimerRef.current);
    copyTimerRef.current = setTimeout(() => setCopiedId(null), 2000);
  };

  const handleDelete = async () => {
    if (!deletingVariable) return;
    try {
      setDeletePending(true);
      await deleteEnvironmentVariable(projectId, serviceId, deletingVariable.id);
      setDeletingVariable(null);
      await loadVariables();
    } catch {
      setActionError('Failed to delete variable. Please try again.');
    } finally {
      setDeletePending(false);
    }
  };

  const environments: { value: Environment; label: string }[] = [
    { value: 'production', label: 'Production' },
    { value: 'preview', label: 'Preview' },
    { value: 'development', label: 'Development' },
  ];

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-3">
          <label htmlFor="env-select" className="text-sm font-medium">
            Environment
          </label>
          <select
            id="env-select"
            value={environment}
            onChange={(e) => setEnvironment(e.target.value as Environment)}
            className="h-9 rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-xs outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
          >
            {environments.map((env) => (
              <option key={env.value} value={env.value}>
                {env.label}
              </option>
            ))}
          </select>
        </div>
        <Button
          size="sm"
          onClick={() => {
            setShowCreateForm(!showCreateForm);
            setEditingVariable(null);
          }}
        >
          <Plus data-icon="inline-start" />
          Add Variable
        </Button>
      </div>

      {actionError ? (
        <p role="alert" className="text-sm text-destructive">
          {actionError}
        </p>
      ) : null}

      {showCreateForm ? (
        <CreateVariableForm
          projectId={projectId}
          serviceId={serviceId}
          environment={environment}
          onSuccess={() => {
            void loadVariables();
            setShowCreateForm(false);
          }}
          onCancel={() => setShowCreateForm(false)}
        />
      ) : null}

      {editingVariable ? (
        <EditVariableForm
          projectId={projectId}
          serviceId={serviceId}
          variable={editingVariable}
          onSuccess={() => {
            void loadVariables();
            setEditingVariable(null);
          }}
          onCancel={() => setEditingVariable(null)}
        />
      ) : null}

      {loading ? (
        <div aria-label="Loading environment variables" className="flex flex-col gap-2">
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
        </div>
      ) : loadError ? (
        <div className="flex flex-col items-start gap-3 rounded-lg border p-5">
          <p role="alert" className="text-sm text-destructive">{loadError}</p>
          <Button type="button" variant="outline" size="sm" onClick={() => void loadVariables()}>
            Retry
          </Button>
        </div>
      ) : variables.length === 0 ? (
        <Empty className="min-h-48 border">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <Variable aria-hidden="true" />
            </EmptyMedia>
            <EmptyTitle>No environment variables</EmptyTitle>
            <EmptyDescription>
              Add your first variable for {environment} to configure this service.
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <div className="flex flex-col gap-2">
          {variables.map((variable) => (
            <div
              key={variable.id}
              className="flex flex-col gap-3 rounded-lg border p-4 sm:flex-row sm:items-start sm:justify-between"
            >
              <div className="min-w-0 flex-1 space-y-2">
                <div className="flex items-center gap-2">
                  <span className="font-mono text-sm font-semibold">{variable.key}</span>
                  {variable.is_secret ? (
                    <Badge variant="secondary" className="text-xs">Secret</Badge>
                  ) : null}
                </div>
                <div className="text-sm">
                  {variable.is_secret ? (
                    revealedSecrets[variable.id] ? (
                      <div className="flex flex-wrap items-center gap-2">
                        <code className="break-all rounded bg-muted px-2 py-1 text-xs">
                          {revealedSecrets[variable.id]}
                        </code>
                        <Button
                          size="icon"
                          variant="ghost"
                          aria-label="Hide secret"
                          onClick={() => handleHideSecret(variable.id)}
                        >
                          <EyeOff />
                        </Button>
                        <Button
                          size="icon"
                          variant="ghost"
                          aria-label={copiedId === variable.id ? 'Copied' : 'Copy value'}
                          onClick={() => void handleCopy(revealedSecrets[variable.id], variable.id)}
                        >
                          {copiedId === variable.id ? <Check /> : <Copy />}
                        </Button>
                      </div>
                    ) : (
                      <div className="flex items-center gap-2">
                        <span className="text-muted-foreground">••••••••</span>
                        <Button
                          size="icon"
                          variant="ghost"
                          aria-label="Reveal secret"
                          onClick={() => void handleRevealSecret(variable.id)}
                        >
                          <Eye />
                        </Button>
                      </div>
                    )
                  ) : (
                    <div className="flex flex-wrap items-center gap-2">
                      <code className="break-all rounded bg-muted px-2 py-1 text-xs">
                        {variable.value}
                      </code>
                      <Button
                        size="icon"
                        variant="ghost"
                        aria-label={copiedId === variable.id ? 'Copied' : 'Copy value'}
                        onClick={() => void handleCopy(variable.value || '', variable.id)}
                      >
                        {copiedId === variable.id ? <Check /> : <Copy />}
                      </Button>
                    </div>
                  )}
                </div>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                <Button
                  size="icon"
                  variant="ghost"
                  aria-label={`Edit ${variable.key}`}
                  onClick={() => {
                    setEditingVariable(variable);
                    setShowCreateForm(false);
                  }}
                >
                  <Edit2 />
                </Button>
                <Button
                  size="icon"
                  variant="ghost"
                  aria-label={`Delete ${variable.key}`}
                  onClick={() => setDeletingVariable(variable)}
                >
                  <Trash2 />
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}

      <AlertDialog open={deletingVariable !== null} onOpenChange={(open) => { if (!open) setDeletingVariable(null); }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia>
              <TrashIcon aria-hidden="true" />
            </AlertDialogMedia>
            <AlertDialogTitle>Delete {deletingVariable?.key}?</AlertDialogTitle>
            <AlertDialogDescription>
              This will permanently remove this environment variable from {environment}.
              Services using it will lose access on next deploy.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deletePending}>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={deletePending}
              aria-busy={deletePending}
              onClick={(event) => {
                event.preventDefault();
                void handleDelete();
              }}
            >
              {deletePending ? <Spinner data-icon="inline-start" /> : <Trash2 data-icon="inline-start" />}
              {deletePending ? 'Deleting…' : 'Delete Variable'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

interface CreateVariableFormProps {
  projectId: string;
  serviceId: string;
  environment: string;
  onSuccess: () => void;
  onCancel: () => void;
}

function CreateVariableForm({
  projectId,
  serviceId,
  environment,
  onSuccess,
  onCancel,
}: CreateVariableFormProps) {
  const [key, setKey] = useState('');
  const [value, setValue] = useState('');
  const [isSecret, setIsSecret] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    try {
      setSubmitting(true);
      await createEnvironmentVariable(projectId, serviceId, {
        key: key.toUpperCase(),
        value,
        is_secret: isSecret,
        environment,
      });
      onSuccess();
    } catch {
      setError('Failed to create variable. Please try again.');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={handleSubmit} className="rounded-lg border p-4" noValidate>
      <FieldGroup className="gap-4">
        {error ? <FieldError>{error}</FieldError> : null}
        <Field>
          <FieldLabel htmlFor="var-key">Key</FieldLabel>
          <Input
            id="var-key"
            placeholder="MY_VARIABLE"
            value={key}
            onChange={(e) => setKey(e.target.value.toUpperCase())}
            required
            autoCapitalize="characters"
            autoCorrect="off"
            spellCheck={false}
          />
          <FieldDescription>Use uppercase letters, numbers, and underscores.</FieldDescription>
        </Field>
        <Field>
          <FieldLabel htmlFor="var-value">Value</FieldLabel>
          <Input
            id="var-value"
            type={isSecret ? 'password' : 'text'}
            placeholder="Enter value"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            required
          />
        </Field>
        <Field>
          <label className="flex cursor-pointer items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={isSecret}
              onChange={(e) => setIsSecret(e.target.checked)}
              className="h-4 w-4 rounded border-input"
            />
            This is a secret (encrypted and hidden by default)
          </label>
        </Field>
      </FieldGroup>
      <div className="mt-4 flex gap-2">
        <Button type="submit" size="sm" disabled={submitting} aria-busy={submitting}>
          {submitting ? <Spinner data-icon="inline-start" /> : <Plus data-icon="inline-start" />}
          {submitting ? 'Creating…' : 'Create Variable'}
        </Button>
        <Button type="button" variant="outline" size="sm" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </form>
  );
}

interface EditVariableFormProps {
  projectId: string;
  serviceId: string;
  variable: EnvironmentVariable;
  onSuccess: () => void;
  onCancel: () => void;
}

function EditVariableForm({
  projectId,
  serviceId,
  variable,
  onSuccess,
  onCancel,
}: EditVariableFormProps) {
  const [value, setValue] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    try {
      setSubmitting(true);
      await updateEnvironmentVariable(projectId, serviceId, variable.id, { value });
      onSuccess();
    } catch {
      setError('Failed to update variable. Please try again.');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form onSubmit={handleSubmit} className="rounded-lg border p-4" noValidate>
      <FieldGroup className="gap-4">
        {error ? <FieldError>{error}</FieldError> : null}
        <p className="text-sm text-muted-foreground">
          {variable.is_secret
            ? `Rotate the secret value for ${variable.key}.`
            : `Update the value for ${variable.key}.`}
        </p>
        <Field>
          <FieldLabel htmlFor="edit-var-value">New Value</FieldLabel>
          <Input
            id="edit-var-value"
            type={variable.is_secret ? 'password' : 'text'}
            placeholder="Enter new value"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            required
          />
        </Field>
      </FieldGroup>
      <div className="mt-4 flex gap-2">
        <Button type="submit" size="sm" disabled={submitting} aria-busy={submitting}>
          {submitting ? <Spinner data-icon="inline-start" /> : null}
          {submitting ? 'Updating…' : 'Update'}
        </Button>
        <Button type="button" variant="outline" size="sm" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </form>
  );
}
