'use client';

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useState, type ReactNode } from 'react';

export function QueryProvider({ children }: { children: ReactNode }) {
  const [client] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            retry: (failureCount, error) => {
              const status =
                typeof error === 'object' && error !== null && 'status' in error
                  ? Number(error.status)
                  : undefined;
              if (status === 401 || status === 429) {
                return false;
              }
              return failureCount < 1;
            },
            staleTime: 5_000,
          },
        },
      }),
  );
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}
