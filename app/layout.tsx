import { Geist, Geist_Mono } from 'next/font/google';

import Script from "next/script";
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
      <head>
        {/*
          react-grab is a dev-only agent tool: it exposes component/file
          structure in the DOM. Off by default; the preview container opts in
          via the NEXT_PUBLIC_REACT_GRAB build arg. Never set it for a real
          production deployment.
        */}
        {(process.env.NODE_ENV === "development" ||
          process.env.NEXT_PUBLIC_REACT_GRAB === "true") && (
          <Script
            src="//unpkg.com/react-grab/dist/index.global.js"
            crossOrigin="anonymous"
            strategy="beforeInteractive"
          />
        )}
      </head>
      <body>{children}</body>
    </html>
  );
}
