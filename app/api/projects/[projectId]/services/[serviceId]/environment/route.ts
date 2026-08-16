import { proxyAPI, withValidatedBody } from '@/lib/api-route';
import {
  createEnvironmentVariableSchema,
  ENVIRONMENT_VARIABLE_ENVIRONMENTS,
} from '@/lib/environment-variable-validation';

type RouteContext = { params: Promise<{ projectId: string; serviceId: string }> };

export async function GET(request: Request, context: RouteContext) {
  const { projectId, serviceId } = await context.params;
  const requested = new URL(request.url).searchParams.get('environment') ?? 'production';
  // Whitelist instead of forwarding: the Go API only ever expects these three.
  const environment = ENVIRONMENT_VARIABLE_ENVIRONMENTS.includes(requested as (typeof ENVIRONMENT_VARIABLE_ENVIRONMENTS)[number])
    ? requested
    : 'production';
  const query = new URLSearchParams({ environment });
  return proxyAPI(
    `/api/v1/projects/${projectId}/services/${serviceId}/environment?${query}`,
    { method: 'GET' },
    'Could not load environment variables.',
  );
}

export async function POST(request: Request, context: RouteContext) {
  const { projectId, serviceId } = await context.params;
  return withValidatedBody(
    request,
    createEnvironmentVariableSchema,
    (data) =>
      proxyAPI(
        `/api/v1/projects/${projectId}/services/${serviceId}/environment`,
        {
          method: 'POST',
          body: JSON.stringify(data),
        },
        'Could not create the environment variable.',
      ),
    { message: 'Check the variable key and value, then try again.' },
  );
}
