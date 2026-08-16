'use client';

import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Copy, Eraser, Play, ShieldCheck, Square, TriangleAlert } from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';

import { FieldDescription, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
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
  createConsoleSession,
  executeConsoleQuery,
  revokeConsoleSession,
  type DatabaseConsoleSession,
  type QueryExecuteRequest,
  type QueryResult,
} from '@/lib/database-console-sessions';
import type { ManagedDatabase } from '@/lib/databases';

type DatabaseConsoleProps = {
  database: ManagedDatabase;
};

const MAX_QUERY_LENGTH = 10_000;
const SESSION_TTL_MINUTES = 30;

function formatSql(sql: string): string {
  const trimmed = sql.trim();
  if (!trimmed) return '';
  const keywords = [
    'SELECT', 'FROM', 'WHERE', 'GROUP BY', 'ORDER BY', 'LIMIT', 'OFFSET',
    'HAVING', 'JOIN', 'LEFT JOIN', 'RIGHT JOIN', 'INNER JOIN', 'OUTER JOIN',
    'AND', 'OR',
  ];
  let formatted = trimmed.replace(/\s+/g, ' ');
  for (const keyword of keywords) {
    const re = new RegExp(`\\b${keyword}\\b`, 'gi');
    formatted = formatted.replace(re, `\n${keyword} `);
  }
  return formatted.replace(/\n\s+/g, '\n').trim();
}

export function DatabaseConsole({ database }: DatabaseConsoleProps) {
  const queryClient = useQueryClient();
  const [session, setSession] = useState<DatabaseConsoleSession | null>(null);
  const [query, setQuery] = useState('');
  const [result, setResult] = useState<QueryResult | null>(null);
  const [copyLabel, setCopyLabel] = useState<'session' | null>(null);
  const copyTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (copyTimerRef.current) clearTimeout(copyTimerRef.current);
    };
  }, []);

  const createSession = useMutation({
    mutationFn: () =>
      createConsoleSession(database.id, { ttlMinutes: SESSION_TTL_MINUTES }),
    onSuccess: (data) => {
      setSession(data);
      void queryClient.invalidateQueries({ queryKey: ['database-console-session'] });
    },
  });

  const execute = useMutation({
    mutationFn: (overrides?: Partial<QueryExecuteRequest>) =>
      executeConsoleQuery(database.id, {
        sessionId: session?.id ?? '',
        query: overrides?.query ?? query,
        disallowMultiStatement: true,
        ...overrides,
      }),
    onSuccess: (data) => setResult(data),
  });

  const revoke = useMutation({
    mutationFn: () => revokeConsoleSession(database.id, session?.id ?? ''),
    onSuccess: () => {
      setSession(null);
      setResult(null);
    },
  });

  const queryTooLong = useMemo(() => query.length > MAX_QUERY_LENGTH, [query]);

  async function copySessionId() {
    if (!session) return;
    await navigator.clipboard.writeText(session.id);
    setCopyLabel('session');
    if (copyTimerRef.current) clearTimeout(copyTimerRef.current);
    copyTimerRef.current = setTimeout(() => setCopyLabel(null), 1500);
  }

  return (
    <div className="flex flex-col gap-8">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <ShieldCheck className="size-5" aria-hidden="true" />
            Console session
          </CardTitle>
          <CardDescription>
            Sessions are short-lived ({SESSION_TTL_MINUTES} minutes), scoped to this
            database only, and can be revoked at any time.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {session ? (
            <div className="flex flex-col gap-4">
              <div className="flex items-center justify-between gap-4 rounded-lg border p-4">
                <div className="min-w-0">
                  <p className="label-meta text-muted-foreground">Active session</p>
                  <p className="mt-1 truncate font-mono text-sm" data-testid="session-id">
                    {session.id}
                  </p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    Expires {new Date(session.expires_at).toLocaleString()}
                  </p>
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => void copySessionId()}
                  >
                    <Copy className="size-4" aria-hidden="true" />
                    {copyLabel === 'session' ? 'Copied' : 'Copy ID'}
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    disabled={revoke.isPending}
                    onClick={() => revoke.mutate()}
                  >
                    Revoke
                  </Button>
                </div>
              </div>
            </div>
          ) : (
            <div className="flex flex-col gap-3">
              <p className="text-sm text-muted-foreground">
                Start a session before running queries. Each query is audited and
                limited to read-only operations.
              </p>
              <div>
                <Button onClick={() => createSession.mutate()} disabled={createSession.isPending}>
                  {createSession.isPending ? (
                    <Spinner className="size-4" aria-hidden="true" />
                  ) : null}
                  {createSession.isPending ? 'Starting session…' : 'Start session'}
                </Button>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <TriangleAlert className="size-5 text-warning" aria-hidden="true" />
            SQL console
          </CardTitle>
          <CardDescription>
            Only single read-only statements are allowed. Writing queries, multiple
            statements and destructive operations are blocked.
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <FieldGroup>
            <FieldLabel htmlFor="sql-query">Query</FieldLabel>
            <FieldDescription>
              One statement, at most {MAX_QUERY_LENGTH.toLocaleString()} characters.
            </FieldDescription>
            <textarea
              id="sql-query"
              data-testid="sql-query-input"
              className="min-h-40 w-full resize-y rounded-lg border bg-background p-3 font-mono text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
              placeholder="SELECT * FROM information_schema.tables LIMIT 10;"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
            />
            {queryTooLong ? (
              <p className="text-sm text-destructive">
                Query exceeds the {MAX_QUERY_LENGTH.toLocaleString()} character limit.
              </p>
            ) : null}
          </FieldGroup>
          <div className="flex flex-wrap items-center gap-2">
            <Button
              disabled={!session || !query.trim() || queryTooLong || execute.isPending}
              onClick={() => execute.mutate(undefined)}
            >
              {execute.isPending ? (
                <Spinner className="size-4" aria-hidden="true" />
              ) : (
                <Play className="size-4" aria-hidden="true" />
              )}
              {execute.isPending ? 'Running…' : 'Run query'}
            </Button>
            <Button
              variant="outline"
              disabled={!session || !query.trim() || queryTooLong || execute.isPending}
              onClick={() =>
                execute.mutate({ query: `EXPLAIN ${query.replace(/;\s*$/, '')}` })
              }
            >
              Explain
            </Button>
            <Button
              variant="outline"
              disabled={!query.trim()}
              onClick={() => setQuery(formatSql(query))}
            >
              Format
            </Button>
            {execute.isPending ? (
              <Button variant="outline" onClick={() => execute.reset()}>
                <Square className="size-4" aria-hidden="true" />
                Cancel
              </Button>
            ) : null}
            <Button
              variant="ghost"
              disabled={!query.trim()}
              onClick={() => {
                setQuery('');
                setResult(null);
              }}
            >
              <Eraser className="size-4" aria-hidden="true" />
              Clear
            </Button>
          </div>

          {execute.isError ? (
            <p className="text-sm text-destructive" data-testid="query-error">
              {(execute.error as Error).message}
            </p>
          ) : null}

          {result ? (
            <div data-testid="query-result">
              {result.rows && result.rows.length > 0 ? (
                <Table>
                  <TableHeader>
                    <TableRow>
                      {(result.columns ?? []).map((column) => (
                        <TableHead key={column}>{column}</TableHead>
                      ))}
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {result.rows.map((row, index) => (
                      <TableRow key={index}>
                        {row.map((cell, cellIndex) => (
                          <TableCell key={cellIndex}>{String(cell)}</TableCell>
                        ))}
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              ) : (
                <p className="text-sm text-muted-foreground">
                  Query completed {result.rows_affected != null ? `· ${result.rows_affected} rows affected` : ''}
                </p>
              )}
            </div>
          ) : null}
        </CardContent>
      </Card>
    </div>
  );
}
