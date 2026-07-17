import '@fontsource/geist-sans/400.css';
import '@fontsource/geist-sans/500.css';
import '@fontsource/geist-sans/600.css';
import '@fontsource/geist-sans/700.css';
import './globals.css';

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
      <body className="font-sans antialiased">{children}</body>
    </html>
  );
}
