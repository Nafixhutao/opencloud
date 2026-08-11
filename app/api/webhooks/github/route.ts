import { NextResponse } from 'next/server';
import { headers } from 'next/headers';

export async function POST(request: Request) {
  try {
    const h = await headers();
    const event = h.get('x-github-event') ?? '';
    const body = await request.json();

    if (event === 'push') {
      const repo = body.repository?.full_name ?? '';
      const branch = (body.ref as string)?.replace('refs/heads/', '') ?? '';
      const sha = body.after ?? '';

      if (!repo || !branch || !sha) {
        return NextResponse.json({ received: true }, { status: 200 });
      }

      // In production, look up matching services by repo URL and trigger builds.
      // For now, acknowledge the webhook.
      console.log('Webhook received:', { event, repo, branch, sha });
    }

    if (event === 'pull_request') {
      const action = body.action;
      const repo = body.repository?.full_name ?? '';
      const branch = body.pull_request?.head?.ref ?? '';
      const sha = body.pull_request?.head?.sha ?? '';

      if (action === 'opened' || action === 'synchronize') {
        console.log('PR webhook — would trigger preview:', { action, repo, branch, sha });
      }

      if (action === 'closed') {
        console.log('PR closed — would destroy preview:', { repo, branch });
      }
    }

    return NextResponse.json({ received: true }, { status: 200 });
  } catch {
    return NextResponse.json({ received: true }, { status: 200 });
  }
}
