'use client';

import { useState, useRef } from 'react';
import { useQuery, useMutation } from '@tanstack/react-query';
import { DownloadIcon, Trash2Icon, UploadIcon, RefreshCwIcon } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card';
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table';
import { Spinner } from '@/components/ui/spinner';
import { Empty, EmptyTitle, EmptyDescription } from '@/components/ui/empty';
import { AlertDialog, AlertDialogContent, AlertDialogHeader, AlertDialogFooter, AlertDialogMedia, AlertDialogTitle, AlertDialogDescription, AlertDialogAction, AlertDialogCancel } from '@/components/ui/alert-dialog';
import { Input } from '@/components/ui/input';

import type { ObjectInfo } from '@/lib/storage';
import { listObjects, uploadObject, deleteObject, getDownloadUrl } from '@/lib/storage';

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
}

type Props = { projectId: string; bucketId: string; bucketName: string };

export function ObjectBrowser({ projectId, bucketId, bucketName }: Props) {
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [deleteTarget, setDeleteTarget] = useState<ObjectInfo | null>(null);
  const [prefix, setPrefix] = useState('');
  const [continuationToken, setContinuationToken] = useState<string | undefined>();
  const [pageTokens, setPageTokens] = useState<string[]>([]);

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: ['storage-objects', projectId, bucketId, prefix, continuationToken],
    queryFn: () => listObjects(projectId, bucketId, { prefix: prefix || undefined, continuationToken, limit: 50 }),
  });

  const objects = data?.data ?? [];
  const nextToken = data?.next_continuation_token;

  const deleteMutation = useMutation({
    mutationFn: (key: string) => deleteObject(projectId, bucketId, key),
    onSuccess: () => {
      setDeleteTarget(null);
      void refetch();
    },
  });

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    try {
      await uploadObject(projectId, bucketId, file.name, file);
      void refetch();
    } catch {
      void refetch();
    }
    if (fileInputRef.current) fileInputRef.current.value = '';
  };

  const handleNextPage = () => {
    if (nextToken) {
      setPageTokens([...pageTokens, continuationToken ?? '']);
      setContinuationToken(nextToken);
    }
  };

  const handlePrevPage = () => {
    const prev = pageTokens.length > 0 ? pageTokens[pageTokens.length - 1] : undefined;
    setPageTokens(pageTokens.slice(0, -1));
    setContinuationToken(prev);
  };

  return (
    <Card size="sm">
      <CardHeader>
        <CardTitle>{bucketName}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="mb-4 flex flex-wrap items-center gap-2">
          <Input
            className="max-w-xs"
            placeholder="Filter by prefix..."
            value={prefix}
            onChange={(e) => { setPrefix(e.target.value); setContinuationToken(undefined); setPageTokens([]); }}
            aria-label="Filter objects by prefix"
          />
          <Button
            size="sm"
            onClick={() => fileInputRef.current?.click()}
          >
            <UploadIcon data-icon="inline-start" />
            Upload File
          </Button>
          <input ref={fileInputRef} type="file" className="hidden" onChange={(e) => void handleUpload(e)} />
          <Button variant="outline" size="sm" onClick={() => void refetch()}>
            <RefreshCwIcon data-icon="inline-start" />
            Refresh
          </Button>
        </div>

        {isLoading ? (
          <div className="space-y-2">
            {[...Array(5)].map((_, i) => (
              <div key={i} className="h-8 w-full animate-pulse rounded bg-muted" />
            ))}
          </div>
        ) : isError ? (
          <div className="py-8 text-center">
            <p className="text-sm text-destructive">Could not load objects.</p>
            <Button variant="outline" size="sm" className="mt-2" onClick={() => void refetch()}>Retry</Button>
          </div>
        ) : objects.length === 0 ? (
          <Empty>
            <EmptyTitle>No objects</EmptyTitle>
            <EmptyDescription>Upload files to start using this bucket.</EmptyDescription>
          </Empty>
        ) : (
          <>
            <div className="md:hidden space-y-2">
              {objects.map((obj) => (
                <div key={obj.key} className="rounded-lg border p-3">
                  <div className="flex items-start justify-between">
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium">{obj.key}</p>
                      <p className="text-xs text-muted-foreground">
                        {formatBytes(obj.size)}{obj.content_type ? ` · ${obj.content_type}` : ''}
                      </p>
                      <p className="text-xs text-muted-foreground">{new Date(obj.last_modified).toLocaleString()}</p>
                    </div>
                    <div className="ml-2 flex gap-1">
                      <a
                        href={getDownloadUrl(projectId, bucketId, obj.key)}
                        className="inline-flex items-center rounded p-1.5 hover:bg-muted"
                        aria-label={`Download ${obj.key}`}
                      >
                        <DownloadIcon size={16} />
                      </a>
                      <button
                        type="button"
                        className="inline-flex items-center rounded p-1.5 text-destructive hover:bg-muted"
                        onClick={() => setDeleteTarget(obj)}
                        aria-label={`Delete ${obj.key}`}
                      >
                        <Trash2Icon size={16} />
                      </button>
                    </div>
                  </div>
                </div>
              ))}
            </div>

            <Table className="hidden md:table">
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Size</TableHead>
                  <TableHead>Type</TableHead>
                  <TableHead>Modified</TableHead>
                  <TableHead className="w-0" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {objects.map((obj) => (
                  <TableRow key={obj.key}>
                    <TableCell className="max-w-60 truncate font-medium">{obj.key}</TableCell>
                    <TableCell>{formatBytes(obj.size)}</TableCell>
                    <TableCell className="max-w-32 truncate text-muted-foreground">{obj.content_type || '-'}</TableCell>
                    <TableCell className="text-muted-foreground">{new Date(obj.last_modified).toLocaleString()}</TableCell>
                    <TableCell>
                      <div className="flex gap-1">
                        <a
                          href={getDownloadUrl(projectId, bucketId, obj.key)}
                          aria-label={`Download ${obj.key}`}
                        >
                          <Button variant="ghost" size="icon-xs">
                            <DownloadIcon size={14} />
                          </Button>
                        </a>
                        <Button variant="ghost" size="icon-xs" onClick={() => setDeleteTarget(obj)} aria-label={`Delete ${obj.key}`}>
                          <Trash2Icon size={14} className="text-destructive" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>

            <div className="mt-4 flex items-center justify-between text-sm">
              <Button variant="outline" size="xs" disabled={pageTokens.length === 0} onClick={handlePrevPage}>
                Previous
              </Button>
              <span className="text-muted-foreground">{objects.length} objects</span>
              <Button variant="outline" size="xs" disabled={!nextToken} onClick={handleNextPage}>
                Next
              </Button>
            </div>
          </>
        )}

        <AlertDialog open={!!deleteTarget} onOpenChange={(open) => { if (!open) setDeleteTarget(null); }}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogMedia />
              <AlertDialogTitle>Delete Object</AlertDialogTitle>
              <AlertDialogDescription>
                Are you sure you want to delete <strong>{deleteTarget?.key}</strong>? This action cannot be undone.
              </AlertDialogDescription>
            </AlertDialogHeader>
            {deleteMutation.error && (
              <p role="alert" className="text-sm text-destructive">
                {deleteMutation.error instanceof Error ? deleteMutation.error.message : 'Deletion failed.'}
              </p>
            )}
            <AlertDialogFooter>
              <AlertDialogCancel onClick={() => setDeleteTarget(null)}>Cancel</AlertDialogCancel>
              <AlertDialogAction
                variant="destructive"
                disabled={deleteMutation.isPending}
                onClick={() => { if (deleteTarget) deleteMutation.mutate(deleteTarget.key); }}
              >
                {deleteMutation.isPending ? <Spinner data-icon="inline-start" /> : null}
                Delete
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </CardContent>
    </Card>
  );
}
