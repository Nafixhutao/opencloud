import type { Metadata } from 'next';
import type { ReactNode } from 'react';

// oxlint-disable-next-line react/only-export-components -- Next.js reads this layout export.
export const metadata: Metadata = {
  title: 'OpenCloud',
  description: 'A modern shared-hosting control plane.',
};

export default function RootLayout({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
