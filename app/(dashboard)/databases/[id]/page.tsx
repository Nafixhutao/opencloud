'use client';

import { useEffect, useState } from 'react';
import { useSearchParams } from 'next/navigation';
import { Terminal, Play, X, Loader2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { DatabaseAPIError, revealDatabaseCredentials } from '@/lib/databases';
import {
  createConsoleSession,
  // type QueryResult,
} from '@/lib/database-console-sessions';
import type { ManagedDatabase } from '@/lib/databases';
import type { DatabaseEngine, DatabaseConsoleSession } from '@/lib/database-console-sessions';

const ENGINE_LABELS: Record<DatabaseEngine, string> = {
  postgres: 'PostgreSQL',
  mariadb: 'MariaDB',
};

export default function DatabaseDetailPage() {
  const searchParams = useSearchParams();
  const [database, setDatabase] = useState<ManagedDatabase | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const tab = searchParams.get('tab') ?? 'overview';

  useEffect(() => {
    const fetchDatabase = async () => {
      try {
        const id = searchParams.get('id');
        if (!id) {
          throw new Error('Missing database ID');
        }

        const response = await fetch(`/api/v1/databases/${id}`);
        if (!response.ok) {
          throw new DatabaseAPIError(
            `Failed to fetch database: ${response.status}`,
            'NOT_FOUND',
            response.status,
          );
        }

        const data = await response.json();
        setDatabase(data.data);
      } catch (err) {
        if (err instanceof DatabaseAPIError) {
          setError(err.message);
        } else {
          setError(err instanceof Error ? err.message : 'Failed to load database');
        }
      } finally {
        setLoading(false);
      }
    };

    fetchDatabase();
  }, [searchParams]);

  if (loading) {
    return (
      <main className="mx-auto flex w-full max-w-[1200px] scroll-mt-20 flex-col gap-8 px-6 py-12 sm:px-8 sm:py-16">
        <div className="flex items-center justify-center py-24">
          <Loader2 className="h-8 w-8 animate-spin text-purple-500" />
        </div>
      </main>
    );
  }

  if (error || !database) {
    return (
      <main className="mx-auto flex w-full max-w-[1200px] scroll-mt-20 flex-col gap-8 px-6 py-12 sm:px-8 sm:py-16">
        <div className="rounded-lg border border-red-200 bg-red-50 p-6">
          <p className="text-red-800">{error || 'Database not found'}</p>
        </div>
      </main>
    );
  }

  return (
    <main className="mx-auto flex w-full max-w-[1200px] scroll-mt-20 flex-col gap-8 px-6 py-12 sm:px-8 sm:py-16">
      <header className="max-w-2xl">
        <div className="flex items-center gap-3">
          <p className="label-meta text-muted-foreground">Resource</p>
          <Badge variant={database.status === 'active' ? 'secondary' : 'default'}>
            {database.status}
          </Badge>
        </div>
        <h1 className="heading-page mt-2">{database.name}</h1>
      </header>

      <nav className="flex gap-1 rounded-t-lg border-b border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-900 p-2">
        <a
          href={`/databases/${database.id}?tab=overview`}
          className={`rounded px-4 py-2 text-sm transition-colors ${
            tab === 'overview'
              ? 'bg-white dark:bg-gray-800 shadow font-medium'
              : ''
          }`}
        >
          Overview
        </a>
        <a
          href={`/databases/${database.id}?tab=console`}
          className={`rounded px-4 py-2 text-sm transition-colors ${
            tab === 'console'
              ? 'bg-white dark:bg-gray-800 shadow font-medium'
              : ''
          }`}
        >
          SQL Console
        </a>
      </nav>

      <OverviewTab database={database} />
      <SQLConsoleTab databaseID={database.id} engine={database.engine} />
    </main>
  );
}

function OverviewTab({ database }: { database: ManagedDatabase }) {
  const [credentials, setCredentials] = useState<{
    host?: string;
    port?: number;
    databaseName?: string;
    username?: string;
    password?: string;
    tlsRequired?: boolean;
  } | null>(null);
  const [loadingCredentials, setLoadingCredentials] = useState(false);
  const [hasRevealed, setHasRevealed] = useState(false);

  const handleReveal = async () => {
    setLoadingCredentials(true);
    try {
      const resp = await revealDatabaseCredentials(database.id);
      setCredentials(resp.data);
      setHasRevealed(true);
    } catch (err) {
      console.error('Failed to reveal credentials:', err);
    } finally {
      setLoadingCredentials(false);
    }
  };

  return (
    <Card className="w-full border-none shadow-none ring-0 focus-within:ring-0">
      <CardContent className="p-6">
        <div className="grid gap-6 md:grid-cols-2">
          <InfoRow label="Engine" value={ENGINE_LABELS[database.engine as DatabaseEngine]} />
          <InfoRow label="Status" value={database.status} />
          <InfoRow label="Created" value={new Date(database.created_at).toLocaleString()} />
          <InfoRow
            label="Credential Available"
            value={database.credential_available ? 'Yes - Not Revealed' : 'No - Previously Revealed'}
          />
        </div>

        {!hasRevealed && (
          <div className="mt-6">
            <Button onClick={handleReveal} disabled={loadingCredentials}>
              {loadingCredentials ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Reveal Credentials
                </>
              ) : (
                'Reveal Connection Details'
              )}
            </Button>
            <p className="mt-2 text-xs text-muted-foreground">
              Warning: Once revealed, credentials cannot be retrieved again. This action is audit-logged.
            </p>
          </div>
        )}

        {hasRevealed && credentials && (
          <div className="mt-6 space-y-4 rounded-lg border bg-gray-50 dark:bg-gray-800 p-4">
            <ConnectionDetails
              engine={database.engine as DatabaseEngine}
              host={credentials.host}
              port={credentials.port}
              database={credentials.databaseName}
              username={credentials.username}
              password={credentials.password}
            />
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                setCredentials(null);
                setHasRevealed(false);
              }}
            >
              Hide Credentials
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p className="text-sm text-muted-foreground">{label}</p>
      <p className="mt-1 font-medium">{value}</p>
    </div>
  );
}

function ConnectionDetails({
  engine,
  host,
  port,
  database,
  username,
  password,
}: {
  engine: DatabaseEngine;
  host?: string;
  port?: number;
  database?: string;
  username?: string;
  password?: string;
}) {
  return (
    <div className="space-y-3">
      <ConnectionLine engine={engine} host={host} port={port} database={database} />
      <CopyableField label="Username" value={username || ''} />
      <CopyableField label="Password" value={password || ''} sensitive />
    </div>
  );
}

function ConnectionLine({
  engine,
  host,
  port,
  database,
}: {
  engine: DatabaseEngine;
  host?: string;
  port?: number;
  database?: string;
}) {
  if (!host || !port || !database) return null;
  const connectionString = `${engine}://${host}:${port}/${database}`;
  return (
    <div>
      <p className="text-sm text-muted-foreground">Connection String</p>
      <code className="mt-1 block break-all rounded bg-gray-100 dark:bg-gray-700 p-2 text-xs">
        {connectionString}
      </code>
    </div>
  );
}

function CopyableField({
  label,
  value,
  sensitive,
}: {
  label: string;
  value: string;
  sensitive?: boolean;
}) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    if (!value) return;
    await navigator.clipboard.writeText(value);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="flex items-center justify-between rounded bg-white dark:bg-gray-900 p-2">
      <span className="text-sm text-muted-foreground">{label}</span>
      <div className="flex items-center gap-2">
        <code className="font-mono">{sensitive ? '•'.repeat(Math.min(value.length, 10)) : value}</code>
        {value && (
          <Button variant="ghost" size="sm" onClick={handleCopy} disabled={!value}>
            {copied ? 'Copied!' : 'Copy'}
          </Button>
        )}
      </div>
    </div>
  );
}

function SQLConsoleTab({ databaseID, _engine }: { databaseID: string; engine: string }) {
  const [session, setSession] = useState<DatabaseConsoleSession | null>(null);
  const [creatingSession, setCreatingSession] = useState(false);
  const [query, setQuery] = useState('SELECT 1');
  const [result, setResult] = useState<any[] | null>(null);
  const [loadingResult, setLoadingResult] = useState(false);
  const [errorResult, setErrorResult] = useState<string | null>(null);
  const [cancelling, setCancelling] = useState(false);

  const ensureSession = async () => {
    if (session) {
      return session;
    }
    setCreatingSession(true);
    try {
      const resp = await createConsoleSession(databaseID, 30);
      setSession(resp.data);
    } catch (err) {
      console.error('Failed to create session:', err);
      // Session might already exist
    } finally {
      setCreatingSession(false);
    }
  };

  const executeQuery = async () => {
    if (!session) {
      await ensureSession();
      if (!session) return;
    }

    if (cancelling) {
      setCancelling(false);
      return;
    }

    setLoadingResult(true);
    setErrorResult(null);
    setResult(null);

    try {
      // Mock results - actual implementation would call backend API
      const mockResults = [
        { col1: 1, col2: 'Test' },
        { col1: 2, col2: 'Data' },
        { col1: 3, col2: 'Ready' },
      ];

      setResult(mockResults);
    } catch (err) {
      setErrorResult(err instanceof Error ? err.message : 'Query execution failed');
    } finally {
      setLoadingResult(false);
      setCancelling(false);
    }
  };

  const cancelQuery = () => {
    setCancelling(true);
    setTimeout(() => setLoadingResult(false), 100);
  };

  const clearResults = () => {
    setResult(null);
    setErrorResult(null);
  };

  return (
    <Card className="w-full border-none shadow-none ring-0 focus-within:ring-0">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Terminal className="h-5 w-5 text-purple-500" />
          SQL Console (READ-ONLY)
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="rounded-lg border bg-gray-50 dark:bg-gray-900 p-4">
          <p className="mb-2 text-sm text-muted-foreground">
            Current session expires in 30 minutes. Query execution is read-only.
          </p>
          <div className="flex items-center gap-2">
            {creatingSession ? (
              <span className="flex items-center text-sm text-muted-foreground">
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Creating secure console session...
              </span>
            ) : session ? (
              <Badge variant="secondary">Active Console Session</Badge>
            ) : (
              <Badge variant="default">No Active Session</Badge>
            )}
          </div>
        </div>

        <div className="space-y-2">
          <label htmlFor="sql-query" className="block text-sm font-medium">
            SQL Query
          </label>
          <textarea
            id="sql-query"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            rows={6}
            className="w-full resize-none rounded-lg border bg-white p-4 font-mono text-sm outline-none focus:ring-2 focus:ring-purple-500 dark:bg-gray-800"
            placeholder="SELECT * FROM your_table;"
          />
        </div>

        <div className="flex gap-2">
          {!loadingResult && !cancelling ? (
            <Button onClick={executeQuery} className="gap-2">
              <Play className="h-4 w-4" /> Run Query
            </Button>
          ) : (
            <Button
              onClick={cancelQuery}
              variant="destructive"
              disabled={cancelling}
              className="gap-2"
            >
              <X className="h-4 w-4" /> Cancel
            </Button>
          )}
          {result && (
            <Button variant="outline" onClick={clearResults}>
              Clear Results
            </Button>
          )}
        </div>

        {errorResult && (
          <div className="rounded-lg border border-red-200 bg-red-50 p-4 text-red-800">
            <p>Error: {errorResult}</p>
          </div>
        )}

        {result && (
          <div className="overflow-x-auto rounded-lg border bg-white dark:bg-gray-800">
            <table className="min-w-full divide-y divide-gray-200">
              <thead className="bg-gray-50 dark:bg-gray-700">
                <tr>
                  {Object.keys(result[0]).map((key) => (
                    <th key={key} className="px-4 py-2 text-left text-xs font-medium uppercase text-gray-500">
                      {key}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y divide-gray-200 dark:divide-gray-700">
                {result.map((row, idx) => (
                  <tr key={idx} className="hover:bg-gray-50 dark:hover:bg-gray-700">
                    {Object.values(row).map((val, i) => (
                      <td key={i} className="px-4 py-2 whitespace-nowrap text-sm">
                        {String(val)}
                      </td>
                    ))}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
