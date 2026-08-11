import { proxyAPI, withValidatedBody } from '@/lib/api-route';
import { createProjectSchema } from '@/lib/project-validation';

export function GET() {
  return proxyAPI('/api/v1/projects', { method: 'GET' }, 'Could not load projects.');
}

export function POST(request: Request) {
  return withValidatedBody(
    request,
    createProjectSchema,
    (data) =>
      proxyAPI(
        '/api/v1/projects',
        {
          method: 'POST',
          headers: { 'Idempotency-Key': request.headers.get('Idempotency-Key') ?? crypto.randomUUID() },
          body: JSON.stringify(data),
        },
        'Could not create project.',
      ),
    { message: 'Check the project details and try again.' },
  );
}
