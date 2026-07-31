import { useQuery } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { QueryProvider } from '@/components/providers/query-provider';
import { DomainAPIError } from '@/lib/domains';

function FailureProbe({
  error,
  attempt,
}: {
  error: Error;
  attempt: () => void;
}) {
  const query = useQuery({
    queryKey: ['production-retry-probe', error.message],
    queryFn: async () => {
      attempt();
      throw error;
    },
  });
  return <p>{query.isError ? 'failed' : 'loading'}</p>;
}

afterEach(cleanup);

describe('QueryProvider retry policy', () => {
  for (const status of [401, 429]) {
    it(`does not retry HTTP ${status}`, async () => {
      const attempt = vi.fn();
      render(
        <QueryProvider>
          <FailureProbe
            error={new DomainAPIError('request stopped', 'STOPPED', status)}
            attempt={attempt}
          />
        </QueryProvider>,
      );

      expect(await screen.findByText('failed')).toBeInTheDocument();
      await waitFor(() => expect(attempt).toHaveBeenCalledTimes(1));
    });
  }
});
