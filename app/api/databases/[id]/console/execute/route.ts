import { z } from 'zod';

import { proxyAPI } from '@/lib/api-route';

/**
 * BFF policy gate for SQL console execution (ADR 0006: BFF owns frontend-
 * facing policy). The Go backend validates too, but this route is the seam
 * where the BFF enforces the contract its own client type advertises —
 * previously a shallow pass-through that sent untrusted JSON straight through.
 *
 * Defaults clamp to server-safe ceilings; disallowMultiStatement defaults to
 * true so a client that omits it cannot slip a multi-statement injection past.
 */
const consoleQuerySchema = z.object({
  sessionId: z.string().min(1, 'sessionId is required'),
  query: z.string().min(1, 'query is required').max(100_000, 'query too long'),
  maxRows: z.number().int().positive().max(1000).optional(),
  timeoutSeconds: z.number().int().positive().max(60).optional(),
  disallowMultiStatement: z.boolean().optional(),
});

type RouteContext = { params: Promise<{ id: string }> };

export async function POST(request: Request, context: RouteContext) {
  const { id } = await context.params;
  const body = await request.json().catch(() => null);

  const parsed = consoleQuerySchema.safeParse(body);
  if (!parsed.success) {
    return Response.json(
      {
        error: {
          code: 'VALIDATION_FAILED',
          message: 'Invalid query execution request',
          details: parsed.error.issues.map((issue) => ({
            field: issue.path.join('.'),
            issue: issue.message,
          })),
        },
      },
      { status: 422 },
    );
  }

  // Enforce safe defaults at the BFF seam — the client type promises these,
  // now the BFF makes them load-bearing.
  const payload = {
    sessionId: parsed.data.sessionId,
    query: parsed.data.query,
    maxRows: parsed.data.maxRows ?? 1000,
    timeoutSeconds: parsed.data.timeoutSeconds ?? 30,
    disallowMultiStatement: parsed.data.disallowMultiStatement ?? true,
  };

  return proxyAPI(
    `/api/v1/databases/${id}/console/execute`,
    {
      method: 'POST',
      cache: 'no-store',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    },
    'Could not execute the query.',
  );
}
