import '@fontsource-variable/inter';
import '@fontsource/jetbrains-mono/400.css';
import '@fontsource/jetbrains-mono/500.css';
import './globals.css';

import type { Metadata } from 'next';
import type { ReactNode } from 'react';

// oxlint-disable-next-line react/only-export-components -- Next.js reads this layout export.
export const metadata: Metadata = {
  title: {
    default: 'Cevra',
    template: '%s · Cevra',
  },
  description: 'Focused cloud hosting operations from one control plane.',
};

export default function RootLayout({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
