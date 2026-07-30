'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  CircleAlert,
  CircleCheck,
  Clipboard,
  Clock3,
  Globe,
  Plus,
  RefreshCw,
  ShieldCheck,
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
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
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
import { attachDomainSchema, type AttachDomainValues } from '@/lib/domain-validation';
import {
  attachDomain,
  detachDomain,
  DomainAPIError,
  getDomainInstructions,
  isDomainTransitional,
  listDomains,
  verifyDomain,
  type DNSRecord,
  type Domain,
  type VerificationInstructions,
} from '@/lib/domains';

type DomainManagerProps = {
  siteID: string;
};

const statusLabel: Record<Domain['status'], string> = {
  pending: 'Pending',
  verifying: 'Verifying',
  verified: 'Verified',
  active: 'Active',
  failed: 'Failed',
  deleting: 'Deleting',
  deleted: 'Deleted',
};

const certStatusLabel: Record<Domain['cert_status'], string> = {
  none: 'No certificate',
  issuing: 'Issuing',
  active: 'Active',
  expiring: 'Expiring',
  revoked: 'Revoked',
  error: 'Error',
};

export function DomainManager({ siteID }: DomainManagerProps) {
  const queryClient = useQueryClient();
  const form = useForm<AttachDomainValues>({
    resolver: zodResolver(attachDomainSchema),
    defaultValues: { hostname: '' },
  });

  const domainsQuery = useQuery({
    queryKey: ['domains', siteID],
    queryFn: () => listDomains(siteID),
    refetchInterval: (query) => {
      const data = query.state.data;
      if (!data) return false;
      return data.data.some((d) => isDomainTransitional(d.status)) ? 3_000 : false;
    },
  });

  const attachMutation = useMutation({
    mutationFn: ({ hostname }: { hostname: string }) => attachDomain(siteID, hostname),
    onSuccess: async () => {
      form.reset();
      await queryClient.invalidateQueries({ queryKey: ['domains', siteID] });
    },
  });

  const verifyMutation = useMutation({
    mutationFn: ({ domainID }: { domainID: string }) => verifyDomain(domainID),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['domains', siteID] });
    },
  });

  const detachMutation = useMutation({
    mutationFn: ({ domainID }: { domainID: string }) => detachDomain(domainID),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['domains', siteID] });
    },
  });

  async function onAttach(values: AttachDomainValues) {
    await attachMutation.mutateAsync({ hostname: values.hostname });
  }

  const mutationError = attachMutation.error ?? verifyMutation.error ?? detachMutation.error;
  const errorMessage =
    mutationError instanceof DomainAPIError
      ? mutationError.message
      : mutationError
        ? 'The control plane could not complete the request.'
        : null;

  return (
    <div className="flex flex-col gap-8">
      <Card>
        <CardHeader>
          <CardTitle>Attach a domain</CardTitle>
          <CardDescription>
            Add a custom hostname you control. You&apos;ll receive TXT verification
            instructions to prove ownership.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={form.handleSubmit(onAttach)} noValidate>
            <FieldGroup className="max-w-2xl gap-4">
              {errorMessage ? <FieldError>{errorMessage}</FieldError> : null}
              <Field data-invalid={Boolean(form.formState.errors.hostname)}>
                <FieldLabel htmlFor="domain-hostname">Hostname</FieldLabel>
                <Input
                  id="domain-hostname"
                  placeholder="www.example.com"
                  autoCapitalize="none"
                  autoCorrect="off"
                  spellCheck={false}
                  aria-invalid={Boolean(form.formState.errors.hostname)}
                  {...form.register('hostname')}
                />
                <FieldDescription>
                  Use an ASCII hostname you control. Universal TXT verification works with any DNS
                  provider.
                </FieldDescription>
                <FieldError errors={[form.formState.errors.hostname]} />
              </Field>
              <div>
                <Button
                  type="submit"
                  disabled={attachMutation.isPending}
                  aria-busy={attachMutation.isPending}
                >
                  {attachMutation.isPending ? (
                    <Spinner data-icon="inline-start" />
                  ) : (
                    <Plus data-icon="inline-start" />
                  )}
                  {attachMutation.isPending ? 'Queueing…' : 'Attach domain'}
                </Button>
              </div>
            </FieldGroup>
          </form>
        </CardContent>
      </Card>

      <section aria-labelledby="domains-heading" className="flex flex-col gap-4">
        <div className="flex items-end justify-between gap-4">
          <div>
            <p className="label-meta text-muted-foreground">Hosting</p>
            <h2 id="domains-heading" className="heading-section mt-1">
              Domains
            </h2>
          </div>
        </div>

        {domainsQuery.isLoading ? (
          <Empty className="min-h-32 border">
            <EmptyHeader>
              <EmptyTitle>Loading…</EmptyTitle>
            </EmptyHeader>
          </Empty>
        ) : domainsQuery.data && domainsQuery.data.data.length === 0 ? (
          <Empty className="min-h-48 border">
            <EmptyHeader>
              <EmptyMedia variant="icon">
                <Globe aria-hidden="true" />
              </EmptyMedia>
              <EmptyTitle>No custom domains</EmptyTitle>
              <EmptyDescription>
                Attach a hostname above to start the verification flow.
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          <div className="flex flex-col gap-4">
            {domainsQuery.data?.data.map((domain) => (
              <DomainCard
                key={domain.id}
                domain={domain}
                verifying={
                  verifyMutation.isPending && verifyMutation.variables?.domainID === domain.id
                }
                detaching={
                  detachMutation.isPending && detachMutation.variables?.domainID === domain.id
                }
                onVerify={(id) => verifyMutation.mutateAsync({ domainID: id })}
                onDetach={(id) => detachMutation.mutateAsync({ domainID: id })}
              />
            ))}
          </div>
        )}
        {domainsQuery.data?.data.some((d) => isDomainTransitional(d.status)) ? (
          <p role="status" className="flex items-center gap-2 text-sm text-muted-foreground">
            <Spinner />
            Refreshing verification status…
          </p>
        ) : null}
      </section>
    </div>
  );
}

function DomainCard({
  domain,
  verifying,
  detaching,
  onVerify,
  onDetach,
}: {
  domain: Domain;
  verifying: boolean;
  detaching: boolean;
  onVerify: (id: string) => Promise<unknown>;
  onDetach: (id: string) => Promise<unknown>;
}) {
  const [instructions, setInstructions] = useState<VerificationInstructions | null>(null);
  const [instructionsLoading, setInstructionsLoading] = useState(false);

  async function loadInstructions() {
    setInstructionsLoading(true);
    try {
      const result = await getDomainInstructions(domain.id);
      setInstructions(result.data);
    } catch {
      // Silently fail; the user can retry
    } finally {
      setInstructionsLoading(false);
    }
  }

  const pending = isDomainTransitional(domain.status);
  const hasCert = domain.cert_status !== 'none';
  const showVerify =
    domain.status === 'pending' ||
    domain.status === 'verifying' ||
    domain.status === 'failed';
  const showDetach = !['deleting', 'deleted'].includes(domain.status);

  return (
    <Card>
      <CardContent className="flex flex-col gap-4 pt-6">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <p className="truncate font-medium">{domain.hostname}</p>
            <p className="mt-1 truncate font-mono text-xs text-muted-foreground">
              {domain.id}
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <DomainStatusBadge domain={domain} />
            {hasCert ? <CertBadge domain={domain} /> : null}
          </div>
        </div>

        {domain.status === 'failed' && domain.last_error ? (
          <p className="text-sm text-destructive">Verification could not complete. Retry below.</p>
        ) : null}

        {(domain.status === 'pending' || domain.status === 'verifying') && !instructions ? (
          <div className="rounded-lg border bg-muted/20 p-4">
            <p className="text-sm text-muted-foreground">
              Verification instructions are available after the domain is queued. Load them to see
              the TXT record you need to add.
            </p>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="mt-3"
              disabled={instructionsLoading}
              onClick={() => void loadInstructions()}
            >
              {instructionsLoading ? <Spinner data-icon="inline-start" /> : null}
              Show instructions
            </Button>
          </div>
        ) : null}

        {instructions ? (
          <VerificationPanel instructions={instructions} domainID={domain.id} onVerify={onVerify} />
        ) : null}

        <div className="flex flex-wrap items-center gap-2">
          {showVerify ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={verifying}
              onClick={() => void onVerify(domain.id)}
            >
              {verifying ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <RefreshCw data-icon="inline-start" />
              )}
              {verifying ? 'Checking…' : 'Verify now'}
            </Button>
          ) : null}
          {showDetach ? (
            <DetachDomainDialog
              domain={domain}
              pending={detaching}
              onDetach={() => onDetach(domain.id)}
            />
          ) : null}
        </div>
      </CardContent>
    </Card>
  );
}

function VerificationPanel({
  instructions,
  domainID: _domainID,
  onVerify: _onVerify,
}: {
  instructions: VerificationInstructions;
  domainID: string;
  onVerify: (id: string) => Promise<unknown>;
}) {
  return (
    <div className="rounded-lg border p-4">
      <div className="flex items-center gap-2">
        <ShieldCheck className="size-4 text-muted-foreground" />
        <span className="text-sm font-medium">Verification instructions</span>
      </div>
      <p className="mt-2 text-sm text-muted-foreground">
        Add these records to your DNS zone at your provider. After adding them,
        click <strong>Verify now</strong> below.
      </p>
      <div className="mt-3 space-y-3">
        {instructions.records.map((record, index) => (
          <RecordLine key={index} record={record} />
        ))}
      </div>
    </div>
  );
}

function RecordLine({ record }: { record: DNSRecord }) {
  const [copied, setCopied] = useState(false);

  function copy() {
    void navigator.clipboard.writeText(record.content).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2_000);
    });
  }

  return (
    <div className="flex flex-col gap-1 rounded bg-muted/40 p-3 font-mono text-xs sm:flex-row sm:items-center sm:justify-between">
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <Badge variant="secondary" className="text-xs">
          {record.type}
        </Badge>
        <span className="text-muted-foreground">{record.name}</span>
      </div>
      <div className="flex items-center gap-2">
        <code className="max-w-[300px] truncate">{record.content}</code>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className="h-7"
          onClick={copy}
          aria-label={copied ? 'Copied' : 'Copy to clipboard'}
        >
          <Clipboard className="size-3" />
          <span className="ml-1 text-xs">{copied ? 'Copied!' : 'Copy'}</span>
        </Button>
      </div>
    </div>
  );
}

function DomainStatusBadge({ domain }: { domain: Domain }) {
  const pending = isDomainTransitional(domain.status);
  const Icon =
    domain.status === 'active'
      ? CircleCheck
      : domain.status === 'failed'
        ? CircleAlert
        : Clock3;
  const variant =
    domain.status === 'failed'
      ? 'destructive'
      : domain.status === 'active'
        ? 'default'
        : 'secondary';
  return (
    <Badge variant={variant}>
      {pending ? <Spinner data-icon="inline-start" /> : <Icon data-icon="inline-start" />}
      {statusLabel[domain.status]}
    </Badge>
  );
}

function CertBadge({ domain }: { domain: Domain }) {
  const variant =
    domain.cert_status === 'error' || domain.cert_status === 'revoked'
      ? 'destructive'
      : domain.cert_status === 'expiring'
        ? 'warning'
        : 'default';
  return (
    <Badge variant={variant}>
      <ShieldCheck data-icon="inline-start" className="size-3" />
      {certStatusLabel[domain.cert_status]}
    </Badge>
  );
}

function DetachDomainDialog({
  domain,
  pending,
  onDetach,
}: {
  domain: Domain;
  pending: boolean;
  onDetach: () => Promise<unknown>;
}) {
  const [open, setOpen] = useState(false);
  const [confirmation, setConfirmation] = useState('');
  const matches = confirmation === domain.hostname;

  function handleOpenChange(next: boolean) {
    setOpen(next);
    if (!next) setConfirmation('');
  }

  async function handleDetach() {
    if (!matches) return;
    await onDetach();
    handleOpenChange(false);
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
            This removes the domain and any associated DNS records. Type the hostname to
            confirm.
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
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>Cancel</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={!matches || pending}
            aria-busy={pending}
            onClick={() => void handleDetach()}
          >
            {pending ? <Spinner data-icon="inline-start" /> : <Trash2 data-icon="inline-start" />}
            {pending ? 'Queueing…' : 'Detach domain'}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}