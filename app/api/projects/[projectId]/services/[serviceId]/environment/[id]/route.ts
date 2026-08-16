import { proxyAPI, withValidatedBody } from '@/lib/api-route';
import { updateEnvironmentVariableSchema } from '@/lib/environment-variable-validation';

type RouteContext = { params: Promise<{ projectId: string; serviceId: string; id: string }> };

export async function PUT(request: Request, context: RouteContext) {
  const { projectId, serviceId, id } = await context.params;
  return withValidatedBody(
    request,
    updateEnvironmentVariableSchema,
    (data) =>
      proxyAPI(
        `/api/v1/projects/${projectId}/services/${serviceId}/environment/${id}`,
        {
          method: 'PUT',
          body: JSON.stringify(data),
        },
        'Could not update the environment variable.',
      ),
    { message: 'Enter a value before saving.' },
  );
}

export async function DELETE(_request: Request, context: RouteContext) {
  const { projectId, serviceId, id } = await context.params;
  return proxyAPI(
    `/api/v1/projects/${projectId}/services/${serviceId}/environment/${id}`,
    { method: 'DELETE' },
    'Could not delete the environment variable.',
  );
}
