import { createHmac, timingSafeEqual } from 'node:crypto';

import { NextResponse } from 'next/server';
import { headers } from 'next/headers';

/**
 * GitHub webhook receiver. Every delivery must carry a valid
 * `x-hub-signature-256` HMAC signed with GITHUB_WEBHOOK_SECRET — an
 * unauthenticated POST must never be able to forge push/PR events.
 * When the secret is not configured the hook is disabled (403) instead of
 * silently accepting unsigned traffic.
 */
function verifySignature(rawBody: string, signatureHeader: string | null, secret: string): boolean {
  if (!signatureHeader?.startsWith('sha256=')) return false;
  const expected = createHmac('sha256', secret).update(rawBody, 'utf8').digest('hex');
  const received = signatureHeader.slice('sha256='.length);
  if (received.length !== expected.length) return false;
  return timingSafeEqual(Buffer.from(expected, 'hex'), Buffer.from(received, 'hex'));
}

export async function POST(request: Request) {
  const secret = process.env.GITHUB_WEBHOOK_SECRET;
  if (!secret) {
    return NextResponse.json(
      { error: { code: 'WEBHOOK_DISABLED', message: 'Webhook secret is not configured' } },
      { status: 403 },
    );
  }

  const rawBody = await request.text();
  const h = await headers();
  if (!verifySignature(rawBody, h.get('x-hub-signature-256'), secret)) {
    return NextResponse.json(
      { error: { code: 'INVALID_SIGNATURE', message: 'Signature verification failed' } },
      { status: 401 },
    );
  }

  try {
    const event = h.get('x-github-event') ?? '';
    const body = JSON.parse(rawBody);

    if (event === 'push') {
      const repo = body.repository?.full_name ?? '';
      const branch = (body.ref as string)?.replace('refs/heads/', '') ?? '';
      const sha = body.after ?? '';

      if (!repo || !branch || !sha) {
        return NextResponse.json({ received: true }, { status: 200 });
      }

      // In production, look up matching services by repo URL and trigger builds.
      // For now, acknowledge the webhook.
      console.info('[webhook] push', { repo, branch, sha });
    }

    if (event === 'pull_request') {
      const action = body.action;
      const repo = body.repository?.full_name ?? '';
      const branch = body.pull_request?.head?.ref ?? '';
      const sha = body.pull_request?.head?.sha ?? '';

      if (action === 'opened' || action === 'synchronize') {
        console.info('[webhook] PR preview', { action, repo, branch, sha });
      }

      if (action === 'closed') {
        console.info('[webhook] PR destroy', { repo, branch });
      }
    }

    return NextResponse.json({ received: true }, { status: 200 });
  } catch {
    return NextResponse.json({ received: true }, { status: 200 });
  }
}
