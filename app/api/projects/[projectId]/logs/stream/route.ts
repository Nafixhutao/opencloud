import { NextResponse } from 'next/server';

import { apiFetch, ApiError } from '@/lib/api';
import { apiErrorToResponse } from '@/lib/api-route';

type RouteContext = { params: Promise<{ projectId: string }> };

/**
 * BFF proxy for the control plane's live log tail (SSE). EventSource cannot
 * set headers, so the browser's session cookie is exchanged for a bearer
 * token here, and the Go API's event stream is piped through untouched —
 * including its 15-second heartbeat, which keeps intermediaries from timing
 * the connection out.
 */
export async function GET(request: Request, context: RouteContext) {
  const { projectId } = await context.params;
  const search = new URL(request.url).searchParams.toString();
  try {
    const upstream = await apiFetch(
      `/api/v1/projects/${projectId}/logs/stream${search ? `?${search}` : ''}`,
      { method: 'GET', headers: { Accept: 'text/event-stream' } },
    );
    if (!upstream.body) {
      return NextResponse.json(
        { error: { code: 'INTERNAL', message: 'Log stream is unavailable.' } },
        { status: 502 },
      );
    }
    return new NextResponse(upstream.body, {
      status: upstream.status,
      headers: {
        'Content-Type': 'text/event-stream; charset=utf-8',
        'Cache-Control': 'no-cache, no-transform',
        Connection: 'keep-alive',
        'X-Accel-Buffering': 'no',
      },
    });
  } catch (error) {
    if (error instanceof ApiError) {
      return apiErrorToResponse(error);
    }
    return NextResponse.json(
      { error: { code: 'INTERNAL', message: 'Log stream is unavailable.' } },
      { status: 502 },
    );
  }
}
