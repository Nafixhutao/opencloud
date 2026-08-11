import { proxyAPI, withValidatedBody } from '@/lib/api-route';
import { attachDomainSchema } from '@/lib/domain-validation';

type RouteContext = { params: Promise<{ id: string }> };

function positiveInteger(value: string | null, fallback: number, maximum: number) {
  if (!value || !/^\d+$/u.test(value)) {
    return fallback;
  }
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) && parsed > 0
    ? Math.min(parsed, maximum)
    : fallback;
}

export async function GET(request: Request, context: RouteContext) {
  const { id } = await context.params;
  const incoming = new URL(request.url);
  const page = positiveInteger(incoming.searchParams.get('page'), 1, 1_000_000);
  const perPage = positiveInteger(incoming.searchParams.get('per_page'), 25, 100);
  const query = page === 1 && perPage === 25 ? '' : `?page=${page}&per_page=${perPage}`;
  return proxyAPI(
    `/api/v1/sites/${id}/domains${query}`,
    { method: 'GET' },
    'Could not load domains.',
  );
}

export async function POST(request: Request, context: RouteContext) {
  const { id } = await context.params;
  return withValidatedBody(
    request,
    attachDomainSchema,
    (data) => {
      const idempotencyKey = request.headers.get('Idempotency-Key') ?? crypto.randomUUID();
      return proxyAPI(
        `/api/v1/sites/${id}/domains`,
        {
          method: 'POST',
          headers: { 'Idempotency-Key': idempotencyKey },
          body: JSON.stringify(data),
        },
        'Could not attach the domain.',
      );
    },
    { message: 'Check the hostname and try again.' },
  );
}
