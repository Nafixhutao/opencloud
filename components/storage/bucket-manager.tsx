'use client';

import { useState } from 'react';
import { useQuery, useMutation } from '@tanstack/react-query';
import { useRouter } from 'next/navigation';
import { FolderPlusIcon } from 'lucide-react';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';

import { Button } from '@/components/ui/button';
import { Card, CardHeader, CardTitle, CardAction, CardContent } from '@/components/ui/card';
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table';
import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Spinner } from '@/components/ui/spinner';
import { Skeleton } from '@/components/ui/skeleton';
import { Empty, EmptyTitle, EmptyDescription } from '@/components/ui/empty';
import { AlertDialog, AlertDialogContent, AlertDialogHeader, AlertDialogFooter, AlertDialogMedia, AlertDialogTitle, AlertDialogDescription, AlertDialogAction, AlertDialogCancel } from '@/components/ui/alert-dialog';
import { FieldGroup, Field, FieldLabel, FieldError } from '@/components/ui/field';

import { listBuckets, createBucket, deleteBucket, hasPendingBuckets, type StorageBucket } from '@/lib/storage';
import { createBucketSchema, type CreateBucketValues } from '@/lib/bucket-validation';

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
}

type Props = { projectId: string; initialData: StorageBucket[] };

export function BucketManager({ projectId, initialData }: Props) {
  const router = useRouter();
  const [page, setPage] = useState(1);
  const [deleteTarget, setDeleteTarget] = useState<StorageBucket | null>(null);
  const [deleteConfirmName, setDeleteConfirmName] = useState('');
  const [showCreateForm, setShowCreateForm] = useState(false);

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ['storage-buckets', projectId, page],
    queryFn: () => listBuckets(projectId, { page, perPage: 25 }),
    initialData: page === 1 ? { data: initialData, meta: { page: 1, per_page: 25, total: initialData.length } } : undefined,
    staleTime: 3000,
  });

  const pending = hasPendingBuckets(data?.data ?? []);
  if (pending) {
    void refetch({ cancelRefetch: false });
  }

  const form = useForm<CreateBucketValues>({
    resolver: zodResolver(createBucketSchema),
    defaultValues: { name: '' },
  });

  const createMutation = useMutation({
    mutationFn: (values: CreateBucketValues) => createBucket(projectId, values.name, crypto.randomUUID()),
    onSuccess: () => {
      setShowCreateForm(false);
      form.reset();
      void refetch();
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (bucketId: string) => deleteBucket(projectId, bucketId),
    onSuccess: () => {
      setDeleteTarget(null);
      setDeleteConfirmName('');
      void refetch();
    },
  });

  const buckets = data?.data ?? [];
  const total = data?.meta?.total ?? 0;
  const perPage = 25;
  const totalPages = Math.ceil(total / perPage);

  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>Object Storage Buckets</CardTitle>
        <CardAction>
          <Button
            size="sm"
            onClick={() => {
              setShowCreateForm(!showCreateForm);
              form.reset();
            }}
          >
            <FolderPlusIcon data-icon="inline-start" />
            New Bucket
          </Button>
        </CardAction>
      </CardHeader>
      <CardContent>
        {showCreateForm && (
          <form
            className="mb-6 rounded-lg border p-4"
            onSubmit={form.handleSubmit((values) => createMutation.mutate(values))}
            noValidate
          >
            <FieldGroup>
              <Field data-invalid={!!form.formState.errors.name}>
                <FieldLabel>Bucket Name</FieldLabel>
                <Input
                  placeholder="my-bucket"
                  {...form.register('name')}
                  aria-describedby={form.formState.errors.name ? 'bucket-name-error' : undefined}
                />
                <FieldError>{form.formState.errors.name?.message}</FieldError>
              </Field>
            </FieldGroup>
            {createMutation.error && (
              <p role="alert" className="mb-3 mt-1 text-sm text-destructive">
                {createMutation.error instanceof Error ? createMutation.error.message : 'Failed to create bucket.'}
              </p>
            )}
            <div className="flex gap-2">
              <Button type="submit" size="sm" disabled={createMutation.isPending}>
                {createMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
                {createMutation.isPending ? 'Queueing...' : 'Create Bucket'}
              </Button>
              <Button type="button" variant="outline" size="sm" onClick={() => setShowCreateForm(false)}>
                Cancel
              </Button>
            </div>
          </form>
        )}

        {createMutation.isSuccess && (
          <p role="status" className="mb-4 text-sm text-success">
            Bucket creation queued. It may take a moment to become active.
          </p>
        )}

        {isLoading && !initialData ? (
          <div className="space-y-2">
            {[...Array(3)].map((_, i) => (<Skeleton key={i} className="h-10 w-full" />))}
          </div>
        ) : isError ? (
          <div className="py-8 text-center">
            <p className="text-sm text-destructive">Could not load buckets.</p>
            <Button variant="outline" size="sm" className="mt-2" onClick={() => void refetch()}>Retry</Button>
          </div>
        ) : buckets.length === 0 && !isLoading ? (
          <Empty>
            <EmptyTitle>No buckets yet</EmptyTitle>
            <EmptyDescription>Create your first object storage bucket to get started.</EmptyDescription>
          </Empty>
        ) : (
          <>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead className="hidden sm:table-cell">Usage</TableHead>
                  <TableHead className="hidden sm:table-cell">Created</TableHead>
                  <TableHead className="w-0" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {buckets.map((bucket) => (
                  <TableRow key={bucket.id}>
                    <TableCell className="font-medium">
                      <button
                        type="button"
                        className="text-left hover:text-link"
                        onClick={() => router.push(`/projects/${projectId}/storage/${bucket.id}`)}
                        aria-label={`Open bucket ${bucket.name}`}
                      >
                        {bucket.name}
                      </button>
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={bucket.status === 'active' ? 'default' : bucket.status === 'failed' ? 'destructive' : 'secondary'}
                      >
                        {bucket.status}
                        {(bucket.status === 'creating' || bucket.status === 'deleting') && (
                          <Spinner data-icon="inline-end" />
                        )}
                      </Badge>
                    </TableCell>
                    <TableCell className="hidden sm:table-cell">
                      <div className="flex items-center gap-2">
                        <div className="h-1.5 w-16 overflow-hidden rounded-full bg-muted">
                          <div
                            className="h-full rounded-full bg-primary transition-all"
                            style={{ width: `${Math.min(100, (bucket.bytes_used / bucket.storage_limit_bytes) * 100)}%` }}
                          />
                        </div>
                        <span className="text-xs text-muted-foreground">
                          {formatBytes(bucket.bytes_used)} / {formatBytes(bucket.storage_limit_bytes)}
                        </span>
                      </div>
                    </TableCell>
                    <TableCell className="hidden text-muted-foreground sm:table-cell">
                      {new Date(bucket.created_at).toLocaleDateString()}
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        variant="destructive"
                        size="xs"
                        onClick={() => {
                          if (bucket.status !== 'active') return;
                          setDeleteTarget(bucket);
                          setDeleteConfirmName('');
                        }}
                        disabled={bucket.status !== 'active'}
                      >
                        Delete
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>

            {totalPages > 1 && (
              <div className="mt-4 flex items-center justify-between text-sm">
                <span className="text-muted-foreground">{total} buckets</span>
                <div className="flex gap-2">
                  <Button variant="outline" size="xs" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
                    Previous
                  </Button>
                  <span className="flex items-center px-2 text-muted-foreground">
                    {page} / {totalPages}
                  </span>
                  <Button variant="outline" size="xs" disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>
                    Next
                  </Button>
                </div>
              </div>
            )}
          </>
        )}
      </CardContent>

      <AlertDialog open={!!deleteTarget} onOpenChange={(open) => { if (!open) { setDeleteTarget(null); setDeleteConfirmName(''); } }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia />
            <AlertDialogTitle>Delete Bucket</AlertDialogTitle>
            <AlertDialogDescription>
              This action cannot be undone. Type <strong>{deleteTarget?.name}</strong> to confirm.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <Input
            value={deleteConfirmName}
            onChange={(e) => setDeleteConfirmName(e.target.value)}
            placeholder={deleteTarget?.name}
            aria-label="Type bucket name to confirm deletion"
          />
          {deleteMutation.error && (
            <p role="alert" className="text-sm text-destructive">
              {deleteMutation.error instanceof Error ? deleteMutation.error.message : 'Deletion failed.'}
            </p>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel onClick={() => { setDeleteTarget(null); setDeleteConfirmName(''); }}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={deleteConfirmName !== deleteTarget?.name || deleteMutation.isPending}
              onClick={() => { if (deleteTarget) deleteMutation.mutate(deleteTarget.id); }}
            >
              {deleteMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
              Delete Bucket
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Card>
  );
}
