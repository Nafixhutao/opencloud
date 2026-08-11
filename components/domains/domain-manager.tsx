'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  CircleAlert,
  CircleCheck,
  ChevronDown,
  Clock3,
  Copy,
  Globe2,
  KeyRound,
  Plus,
  RefreshCw,
  ShieldCheck,
  Trash2,
} from 'lucide-react';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { useCallback, useEffect, useRef, useState } from 'react';
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
import { Separator } from '@/components/ui/separator';
import { Skeleton } from '@/components/ui/skeleton';
import { Spinner } from '@/components/ui/spinner';
import { attachDomainSchema, type AttachDomainValues } from '@/lib/domain-validation';
import {
  attachDomain,
  detachDomain,
  DomainAPIError,
  type Domain,
  type DomainEnvelope,
  type DomainsEnvelope,
  getDomainInstructions,
  hasPendingDomains,
  listDomains,
  rotateDomainChallenge,
  verifyDomain,
} from '@/lib/domains';
import type { Site } from '@/lib/sites';

type DomainManagerProps = {
  site: Site;
  initialData: DomainsEnvelope;
};

type DomainAction = 'verify' | 'rotate' | 'detach';

const statusContent: Record<Domain['status'], { label: string; detail: string }> = {
  pending: {
    label: 'DNS setup',
    detail: 'Add the records below, then check ownership.',
  },
  verifying: {
    label: 'Checking DNS',
    detail: 'Ownership is being checked through public DNS. This may take a few minutes.',
  },
  dns_pending: {
    label: 'DNS verified',
    detail: 'Ownership is verified. Waiting for the routing record to become public.',
  },
  provisioning: {
    label: 'Publishing',
    detail: 'The hostname is being published and HTTPS is being requested.',
  },
  active: {
    label: 'Active',
    detail: 'Traffic is routed and HTTPS is monitored automatically.',
  },
  failed: {
    label: 'Needs attention',
    detail: 'Setup did not complete. Review DNS, then retry.',
  },
  deleting: {
    label: 'Detaching',
    detail: 'Routing and managed DNS state are being removed.',
  },
  deleted: {
    label: 'Detached',
    detail: 'This hostname is no longer attached.',
  },
};

const transientPollBackoffMs = 10_000;

export function DomainManager({ site, initialData }: DomainManagerProps) {
  const queryClient = useQueryClient();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const attachAttempt = useRef<{ hostname: string; key: string } | null>(null);
  const requestedPage = Number(searchParams.get('page') ?? initialData.meta.page);
  const page = Number.isInteger(requestedPage) && requestedPage > 0
    ? requestedPage
    : 1;
  const perPage = initialData.meta.per_page;
  const form = useForm<AttachDomainValues>({
    resolver: zodResolver(attachDomainSchema),
    defaultValues: { hostname: '' },
  });
  const domainsQuery = useQuery({
    queryKey: ['domains', site.id, page, perPage],
    queryFn: () => listDomains(site.id, page, perPage),
    initialData: page === initialData.meta.page ? initialData : undefined,
    refetchInterval: (query) => {
      const error = query.state.error;
      if (error instanceof DomainAPIError) {
        if (error.status === 401) {
          return false;
        }
        if (error.status === 429) {
          return Math.max(2_000, (error.retryAfterSeconds ?? 2) * 1_000);
        }
        if (error.status !== 408 && error.status < 500) {
          return false;
        }
      }
      if (error) {
        return query.state.data && hasPendingDomains(query.state.data.data)
          ? transientPollBackoffMs
          : false;
      }
      return query.state.data && hasPendingDomains(query.state.data.data) ? 2_000 : false;
    },
  });
  const domainPage = domainsQuery.data;
  const total = domainPage?.meta.total ?? initialData.meta.total;
  const totalPages = Math.max(1, Math.ceil(total / perPage));

  const navigatePage = useCallback((nextPage: number, replace = false) => {
    const normalized = Math.max(1, nextPage);
    const next = new URLSearchParams(searchParams.toString());
    if (normalized === 1) {
      next.delete('page');
    } else {
      next.set('page', String(normalized));
    }
    const target = next.size > 0 ? `${pathname}?${next.toString()}` : pathname;
    if (replace) {
      router.replace(target, { scroll: false });
    } else {
      router.push(target, { scroll: false });
    }
  }, [pathname, router, searchParams]);

  useEffect(() => {
    if (page > totalPages) {
      navigatePage(totalPages, true);
    }
  }, [navigatePage, page, totalPages]);

  const attachMutation = useMutation({
    mutationFn: ({ hostname, key }: { hostname: string; key: string }) =>
      attachDomain(site.id, hostname, key),
    onSuccess: async () => {
      attachAttempt.current = null;
      form.reset();
      navigatePage(1, true);
      await queryClient.invalidateQueries({ queryKey: ['domains', site.id] });
    },
  });
  const actionMutation = useMutation({
    mutationFn: ({ domain, action }: { domain: Domain; action: DomainAction }) => {
      if (action === 'verify') {
        return verifyDomain(domain.id);
      }
      if (action === 'rotate') {
        return rotateDomainChallenge(domain.id);
      }
      return detachDomain(domain.id);
    },
    onSuccess: async (response: DomainEnvelope, variables) => {
      queryClient.setQueryData<DomainsEnvelope>(['domains', site.id, page, perPage], (current) =>
        current
          ? {
              ...current,
              data: current.data.map((domain) =>
                domain.id === response.data.id ? response.data : domain,
              ),
            }
          : current,
      );
      if (variables.action === 'rotate') {
        await queryClient.invalidateQueries({
          queryKey: ['domain-instructions', variables.domain.id],
        });
      }
      await queryClient.invalidateQueries({ queryKey: ['domains', site.id] });
    },
  });

  useEffect(() => {
    const unauthorized = [
      domainsQuery.error,
      attachMutation.error,
      actionMutation.error,
    ].some((error) => error instanceof DomainAPIError && error.status === 401);
    if (unauthorized) {
      router.replace(sessionExpiredTarget(pathname, searchParams));
    }
  }, [
    actionMutation.error,
    attachMutation.error,
    domainsQuery.error,
    pathname,
    router,
    searchParams,
  ]);

  async function onAttach(values: AttachDomainValues) {
    const hostname = values.hostname.trim().toLowerCase();
    if (!attachAttempt.current || attachAttempt.current.hostname !== hostname) {
      attachAttempt.current = { hostname, key: crypto.randomUUID() };
    }
    try {
      await attachMutation.mutateAsync(attachAttempt.current);
    } catch (error) {
      if (error instanceof DomainAPIError) {
        const hostnameIssue = error.details.find(
          (detail) => detail.field === 'hostname',
        );
        if (hostnameIssue) {
          form.setError('hostname', { type: 'server', message: hostnameIssue.issue }, {
            shouldFocus: true,
          });
        }
      }
      // TanStack Query owns the displayed mutation error.
    }
  }

  const hasHostnameIssue =
    attachMutation.error instanceof DomainAPIError &&
    attachMutation.error.details.some((detail) => detail.field === 'hostname');
  const attachErrorMessage = hasHostnameIssue ? null : domainErrorMessage(attachMutation.error);
  const canAttach = site.status === 'active';

  return (
    <div className="flex flex-col gap-10">
      <Card>
        <CardHeader>
          <CardTitle>Attach a hostname</CardTitle>
          <CardDescription>
            Use any DNS provider. OpenCloud gives you the exact TXT ownership challenge
            and A record; no administrator step is required.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={form.handleSubmit(onAttach)} noValidate>
            <FieldGroup className="max-w-2xl gap-4">
              {attachErrorMessage ? <FieldError>{attachErrorMessage}</FieldError> : null}
              <Field data-invalid={Boolean(form.formState.errors.hostname)}>
                <FieldLabel htmlFor="domain-hostname">Hostname</FieldLabel>
                <Input
                  id="domain-hostname"
                  placeholder="www.example.com"
                  autoCapitalize="none"
                  autoCorrect="off"
                  autoComplete="off"
                  spellCheck={false}
                  disabled={!canAttach || attachMutation.isPending}
                  aria-invalid={Boolean(form.formState.errors.hostname)}
                  aria-describedby="domain-hostname-description domain-hostname-error"
                  {...form.register('hostname')}
                />
                <FieldDescription id="domain-hostname-description">
                  {canAttach
                    ? 'Enter a hostname only, without https://, a path, wildcard, or port.'
                    : disabledAttachGuidance(site.status)}
                </FieldDescription>
                <FieldError id="domain-hostname-error" errors={[form.formState.errors.hostname]} />
              </Field>
              <div>
                <Button
                  type="submit"
                  disabled={!canAttach || attachMutation.isPending}
                  aria-busy={attachMutation.isPending}
                >
                  {attachMutation.isPending ? (
                    <Spinner data-icon="inline-start" />
                  ) : (
                    <Plus data-icon="inline-start" />
                  )}
                  {attachMutation.isPending ? 'Attaching…' : 'Attach hostname'}
                </Button>
              </div>
            </FieldGroup>
          </form>
        </CardContent>
      </Card>

      <section aria-labelledby="domains-heading" className="flex flex-col gap-4">
        <div className="flex items-end justify-between gap-4">
          <div>
            <p className="label-meta text-muted-foreground">Routing</p>
            <h2 id="domains-heading" className="heading-section mt-1">
              Custom domains
            </h2>
          </div>
          <p className="text-sm text-muted-foreground tabular-nums">
            {total} {total === 1 ? 'domain' : 'domains'}
          </p>
        </div>

        {domainsQuery.isPending ? (
          <div aria-label="Loading domain page" className="flex flex-col gap-2 rounded-lg border p-5">
            <Skeleton className="h-24 w-full" />
            <Skeleton className="h-24 w-full" />
          </div>
        ) : domainsQuery.isError && !domainPage ? (
          <div className="flex flex-col items-start gap-3 rounded-lg border p-5">
            <FieldError>{domainErrorMessage(domainsQuery.error)}</FieldError>
            <Button type="button" variant="outline" onClick={() => void domainsQuery.refetch()}>
              Retry page
            </Button>
          </div>
        ) : domainPage && domainPage.data.length === 0 && total === 0 ? (
          <Empty className="min-h-64 border">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <Globe2 aria-hidden="true" />
              </EmptyMedia>
              <EmptyTitle>No custom domains</EmptyTitle>
              <EmptyDescription>
                Attach a hostname above to start the guided DNS and HTTPS setup.
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          <div className="overflow-hidden rounded-lg border">
            {domainPage?.data.map((domain, index) => (
              <div key={domain.id}>
                {index > 0 ? <Separator /> : null}
                <DomainRow
                  domain={domain}
                  pending={
                    actionMutation.isPending &&
                    actionMutation.variables?.domain.id === domain.id
                  }
                  action={
                    actionMutation.variables?.domain.id === domain.id
                      ? actionMutation.variables.action
                      : undefined
                  }
                  actionError={
                    actionMutation.variables?.domain.id === domain.id
                      ? domainErrorMessage(actionMutation.error)
                      : null
                  }
                  onAction={(action) => {
                    actionMutation.reset();
                    return actionMutation.mutateAsync({ domain, action });
                  }}
                  onClearActionError={() => actionMutation.reset()}
                />
              </div>
            ))}
          </div>
        )}

        {domainPage && hasPendingDomains(domainPage.data) && !domainsQuery.error ? (
          <p role="status" className="flex items-center gap-2 text-sm text-muted-foreground">
            <Spinner />
            Refreshing DNS, routing, and certificate status…
          </p>
        ) : null}
        {domainPage &&
        domainsQuery.error &&
        !(domainsQuery.error instanceof DomainAPIError && [401, 429].includes(domainsQuery.error.status)) ? (
          <div className="flex flex-wrap items-center gap-3 rounded-lg border p-4">
            <FieldError>{domainErrorMessage(domainsQuery.error)}</FieldError>
            <Button type="button" variant="outline" onClick={() => void domainsQuery.refetch()}>
              Retry status
            </Button>
          </div>
        ) : null}
        {domainsQuery.error instanceof DomainAPIError && domainsQuery.error.status === 429 ? (
          <p role="status" className="text-sm text-muted-foreground">
            Updates are paused until the control-plane request budget resets.
          </p>
        ) : null}
        {domainsQuery.error instanceof DomainAPIError && domainsQuery.error.status === 401 ? (
          <p role="alert" className="text-sm text-destructive">
            Your session expired. Sign in again before refreshing domain status.
          </p>
        ) : null}

        {totalPages > 1 ? (
          <nav
            aria-label="Domain pages"
            className="flex flex-col gap-3 border-t pt-4 sm:flex-row sm:items-center sm:justify-between"
          >
            <p className="text-sm text-muted-foreground tabular-nums">
              Page {page} of {totalPages}
            </p>
            <div className="flex gap-2">
              <Button
                type="button"
                variant="outline"
                disabled={page === 1 || domainsQuery.isFetching}
                onClick={() => navigatePage(Math.max(1, page - 1))}
              >
                Previous
              </Button>
              <Button
                type="button"
                variant="outline"
                disabled={page === totalPages || domainsQuery.isFetching}
                onClick={() => navigatePage(Math.min(totalPages, page + 1))}
              >
                Next
              </Button>
            </div>
          </nav>
        ) : null}
      </section>
    </div>
  );
}

function DomainRow({
  domain,
  pending,
  action,
  actionError,
  onAction,
  onClearActionError,
}: {
  domain: Domain;
  pending: boolean;
  action?: DomainAction;
  actionError: string | null;
  onAction: (action: DomainAction) => Promise<unknown>;
  onClearActionError: () => void;
}) {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const [detailsOpen, setDetailsOpen] = useState(false);
  const instructionsQuery = useQuery({
    queryKey: ['domain-instructions', domain.id, domain.updated_at],
    queryFn: () => getDomainInstructions(domain.id),
    enabled: detailsOpen && !['deleting', 'deleted'].includes(domain.status),
    retry: false,
  });
  const status = statusContent[domain.status];
  const isUnverified = !domain.verified_at;

  useEffect(() => {
    if (
      instructionsQuery.error instanceof DomainAPIError &&
      instructionsQuery.error.status === 401
    ) {
      router.replace(sessionExpiredTarget(pathname, searchParams));
    }
  }, [instructionsQuery.error, pathname, router, searchParams]);

  function requestAction(nextAction: DomainAction) {
    void onAction(nextAction).catch(() => {
      // TanStack Query owns the displayed mutation error.
    });
  }

  return (
    <article className="flex flex-col gap-6 p-5 sm:p-6" aria-labelledby={`domain-${domain.id}`}>
      <div className="flex flex-col justify-between gap-4 sm:flex-row sm:items-start">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h3 id={`domain-${domain.id}`} className="truncate text-base font-semibold">
              {domain.hostname}
            </h3>
            <DomainStatusBadge domain={domain} />
          </div>
          <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
            {status.detail}
          </p>
          {domain.status === 'failed' && domain.last_error ? (
            <p role="alert" className="mt-2 text-sm text-destructive">
              {domain.last_error}
            </p>
          ) : null}
          {actionError && action !== 'detach' ? (
            <p role="alert" className="mt-2 text-sm text-destructive">
              {actionError}
            </p>
          ) : null}
        </div>
        <div className="flex shrink-0 flex-wrap gap-2">
          {['pending', 'failed'].includes(domain.status) ? (
            <Button
              type="button"
              size="sm"
              disabled={pending}
              aria-busy={pending && action === 'verify'}
              onClick={() => requestAction('verify')}
            >
              {pending && action === 'verify' ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <ShieldCheck data-icon="inline-start" />
              )}
              {domain.status === 'failed' ? 'Retry setup' : 'Check DNS'}
            </Button>
          ) : null}
          {isUnverified && !['verifying', 'deleting', 'deleted'].includes(domain.status) ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={pending}
              aria-busy={pending && action === 'rotate'}
              onClick={() => requestAction('rotate')}
            >
              {pending && action === 'rotate' ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <RefreshCw data-icon="inline-start" />
              )}
              Rotate challenge
            </Button>
          ) : null}
          {!['deleting', 'deleted'].includes(domain.status) ? (
            <DetachDomainDialog
              domain={domain}
              pending={pending}
              error={action === 'detach' ? actionError : null}
              onDetach={() => onAction('detach')}
              onClearError={onClearActionError}
            />
          ) : null}
        </div>
      </div>

      {!['deleting', 'deleted'].includes(domain.status) ? (
        <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(14rem,0.42fr)]">
          <section aria-labelledby={`dns-${domain.id}`} className="min-w-0">
            <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
              <div>
                <h4 id={`dns-${domain.id}`} className="text-sm font-medium">
                  DNS records
                </h4>
              {isUnverified ? (
                <p className="text-xs text-muted-foreground">
                  Challenge expires {formatDateTime(domain.verification_expires_at)}
                </p>
              ) : (
                <p className="text-xs text-muted-foreground">Ownership verified</p>
              )}
              </div>
              <Button
                type="button"
                size="sm"
                variant="outline"
                aria-expanded={detailsOpen}
                aria-controls={`dns-records-${domain.id}`}
                onClick={() => setDetailsOpen((open) => !open)}
              >
                <ChevronDown
                  aria-hidden="true"
                  className={
                    detailsOpen
                      ? 'rotate-180 transition-transform motion-reduce:transition-none'
                      : 'transition-transform motion-reduce:transition-none'
                  }
                />
                {detailsOpen ? 'Hide DNS records' : 'Show DNS records'}
              </Button>
            </div>
            {detailsOpen ? (
              <div id={`dns-records-${domain.id}`}>
                {instructionsQuery.isPending ? (
                  <div aria-label="Loading DNS records" className="flex flex-col gap-2">
                    <Skeleton className="h-16 w-full" />
                  </div>
                ) : instructionsQuery.error ? (
                  <div className="flex flex-col items-start gap-2">
                    <FieldError>
                      {instructionsQuery.error instanceof DomainAPIError
                        ? instructionsQuery.error.message
                        : 'DNS instructions are temporarily unavailable.'}
                    </FieldError>
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => void instructionsQuery.refetch()}
                    >
                      Retry DNS records
                    </Button>
                  </div>
                ) : (
                  <div className="divide-y rounded-md border bg-muted/20">
                    {instructionsQuery.data?.data.records.map((record) => (
                      <DNSRecordRow
                        key={`${record.type}-${record.name}`}
                        type={record.type}
                        name={record.name}
                        content={record.content}
                        ttl={record.ttl}
                      />
                    ))}
                  </div>
                )}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">
                Open the records only when you are ready to update DNS.
              </p>
            )}
          </section>

          <CertificateSummary domain={domain} />
        </div>
      ) : null}
    </article>
  );
}

function DomainStatusBadge({ domain }: { domain: Domain }) {
  const pending = ['verifying', 'dns_pending', 'provisioning', 'deleting'].includes(domain.status);
  const Icon = domain.status === 'active'
    ? CircleCheck
    : domain.status === 'failed'
      ? CircleAlert
      : Clock3;
  const variant = domain.status === 'failed'
    ? 'destructive'
    : domain.status === 'active'
      ? 'default'
      : 'secondary';
  return (
    <Badge variant={variant} aria-live="polite">
      {pending ? <Spinner data-icon="inline-start" /> : <Icon data-icon="inline-start" />}
      {statusContent[domain.status].label}
    </Badge>
  );
}

function DNSRecordRow({
  type,
  name,
  content,
  ttl,
}: {
  type: string;
  name: string;
  content: string;
  ttl: number;
}) {
  const [copied, setCopied] = useState(false);
  const [copyFailed, setCopyFailed] = useState(false);

  async function copyContent() {
    try {
      await navigator.clipboard.writeText(content);
      setCopied(true);
      setCopyFailed(false);
      window.setTimeout(() => setCopied(false), 1_500);
    } catch {
      setCopied(false);
      setCopyFailed(true);
    }
  }

  return (
    <div className="grid gap-3 p-3 sm:grid-cols-[3rem_minmax(0,0.65fr)_minmax(0,1fr)_auto] sm:items-center">
      <Badge variant="outline">{type}</Badge>
      <div className="min-w-0">
        <p className="text-xs text-muted-foreground">Name</p>
        <p className="truncate font-mono text-xs" title={name}>
          {name}
        </p>
      </div>
      <div className="min-w-0">
        <p className="text-xs text-muted-foreground">Value · TTL {ttl}</p>
        <p className="select-text break-all font-mono text-xs">{content}</p>
        {copyFailed ? (
          <p role="status" className="mt-1 text-xs text-destructive">
            Copy failed. Select the value and copy it manually.
          </p>
        ) : null}
      </div>
      <Button
        type="button"
        size="icon"
        variant="ghost"
        aria-label={copied ? `${type} record value copied` : `Copy ${type} record value`}
        onClick={() => void copyContent()}
      >
        {copied ? <CircleCheck /> : <Copy />}
        <span className="sr-only" aria-live="polite">
          {copied ? 'Copied' : ''}
        </span>
      </Button>
    </div>
  );
}

function CertificateSummary({ domain }: { domain: Domain }) {
  const certificateLabel: Record<Domain['cert_status'], string> = {
    none: 'Not issued',
    issuing: 'Issuing',
    active: 'Active',
    expiring: 'Renewing soon',
    error: 'Needs attention',
  };
  return (
    <section aria-labelledby={`certificate-${domain.id}`} className="rounded-md border p-4">
      <div className="flex items-center justify-between gap-3">
        <h4 id={`certificate-${domain.id}`} className="flex items-center gap-2 text-sm font-medium">
          <KeyRound aria-hidden="true" className="size-4" />
          HTTPS certificate
        </h4>
        <Badge
          variant={domain.cert_status === 'error' ? 'destructive' : 'outline'}
          aria-live="polite"
        >
          {domain.cert_status === 'issuing' ? <Spinner data-icon="inline-start" /> : null}
          {certificateLabel[domain.cert_status]}
        </Badge>
      </div>
      <p className="mt-3 text-sm leading-6 text-muted-foreground">
        {domain.cert_expires_at
          ? `Expires ${formatDateTime(domain.cert_expires_at)}. Automatic renewal is ${domain.cert_auto_renew ? 'on' : 'off'}.`
          : domain.status === 'active'
            ? 'Waiting for the first successful certificate observation.'
            : 'Issuance begins after DNS routing is verified.'}
      </p>
      {domain.cert_observed_at ? (
        <p className="mt-2 text-xs text-muted-foreground">
          Last checked {formatDateTime(domain.cert_observed_at)}
        </p>
      ) : null}
    </section>
  );
}

function DetachDomainDialog({
  domain,
  pending,
  error,
  onDetach,
  onClearError,
}: {
  domain: Domain;
  pending: boolean;
  error: string | null;
  onDetach: () => Promise<unknown>;
  onClearError: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [confirmation, setConfirmation] = useState('');
  const matches = confirmation === domain.hostname;

  function handleOpenChange(next: boolean) {
    if (next && error) {
      onClearError();
    }
    setOpen(next);
    if (!next) {
      setConfirmation('');
    }
  }

  async function handleDetach() {
    if (!matches) {
      return;
    }
    try {
      await onDetach();
      handleOpenChange(false);
    } catch {
      // The controlled dialog stays open and renders the row-scoped error.
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={handleOpenChange}>
      <AlertDialogTrigger render={<Button type="button" size="sm" variant="ghost" />}>
        <Trash2 data-icon="inline-start" />
        Detach
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogMedia>
            <Trash2 aria-hidden="true" />
          </AlertDialogMedia>
          <AlertDialogTitle>Detach {domain.hostname}?</AlertDialogTitle>
          <AlertDialogDescription>
            OpenCloud will stop routing this hostname and remove managed record state. Your
            site and primary hostname stay online. Type the hostname to confirm.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <Field data-invalid={confirmation.length > 0 && !matches}>
          <FieldLabel htmlFor={`detach-${domain.id}`}>Hostname</FieldLabel>
          <Input
            id={`detach-${domain.id}`}
            value={confirmation}
            onChange={(event) => setConfirmation(event.target.value)}
            aria-invalid={confirmation.length > 0 && !matches}
            autoComplete="off"
          />
        </Field>
        {error ? <FieldError>{error}</FieldError> : null}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>Cancel</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={!matches || pending}
            aria-busy={pending}
            onClick={(event) => {
              event.preventDefault();
              void handleDetach();
            }}
          >
            {pending ? <Spinner data-icon="inline-start" /> : <Trash2 data-icon="inline-start" />}
            {pending ? 'Queueing…' : 'Detach domain'}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function domainErrorMessage(error: unknown): string | null {
  if (error instanceof DomainAPIError) {
    return error.message;
  }
  return error ? 'The control plane could not complete the request.' : null;
}

function sessionExpiredTarget(
  pathname: string,
  searchParams: Pick<URLSearchParams, 'toString'>,
): string {
  const query = searchParams.toString();
  const next = query ? `${pathname}?${query}` : pathname;
  return `/login?notice=session-expired&next=${encodeURIComponent(next)}`;
}

function disabledAttachGuidance(status: Site['status']): string {
  const guidance: Record<Site['status'], string> = {
    provisioning: 'Wait for site provisioning to finish before attaching a hostname.',
    active: 'Enter a hostname only, without https://, a path, wildcard, or port.',
    suspending: 'Wait for suspension to finish, then resume the site.',
    suspended: 'Resume this site before attaching another hostname.',
    resuming: 'Wait for the site to finish resuming.',
    deleting: 'This site is being deleted and cannot accept another hostname.',
    deleted: 'This site has been deleted and cannot accept another hostname.',
    failed: 'Delete this failed site and create a healthy site before attaching a hostname.',
  };
  return guidance[status];
}

function formatDateTime(value: string): string {
  return new Intl.DateTimeFormat('en', {
    dateStyle: 'medium',
    timeStyle: 'short',
    timeZone: 'UTC',
  }).format(new Date(value)) + ' UTC';
}
