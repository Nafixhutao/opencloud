import { proxyAPI, withValidatedBody } from '@/lib/api-route';
import { createSiteSchema } from '@/lib/site-validation';

export function GET() {
  return proxyAPI('/api/v1/sites', { method: 'GET' }, 'Could not load sites.');
}

export function POST(request: Request) {
  return withValidatedBody(
    request,
    createSiteSchema,
    (data) => {
      const idempotencyKey = request.headers.get('Idempotency-Key') ?? crypto.randomUUID();
      return proxyAPI(
        '/api/v1/sites',
        {
          method: 'POST',
          headers: { 'Idempotency-Key': idempotencyKey },
          body: JSON.stringify(data),
        },
        'Could not queue site creation.',
      );
    },
    { message: 'Check the site details and try again.' },
  );
}
