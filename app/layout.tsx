import { Geist, Geist_Mono } from 'next/font/google';

import './globals.css';

const geistSans = Geist({
  variable: '--font-geist-sans',
  subsets: ['latin'],
});

const geistMono = Geist_Mono({
  variable: '--font-geist-mono',
  subsets: ['latin'],
});

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
    <html lang="en" className={`${geistSans.variable} ${geistMono.variable}`}>
      <body>{children}</body>
    </html>
  );
}
