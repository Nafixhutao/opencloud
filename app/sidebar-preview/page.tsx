import type { Metadata } from 'next';

import { Sidebar } from '@/planing-ui/sidebar/sidebar';

// oxlint-disable-next-line react/only-export-components -- Next.js reads this page export.
export const metadata: Metadata = {
  title: 'Sidebar preview',
  description: 'Visual reference implementation of the dashboard sidebar.',
};

/**
 * Isolated canvas for the reference sidebar.
 *
 * Deliberately outside the `(dashboard)` route group: that layout already
 * renders the production `<Sidebar>` from `components/navigation`, and nesting
 * this one inside it would show two sidebars at once.
 */
export default function SidebarPreviewPage() {
  return (
    <div className="flex h-screen overflow-hidden bg-[#0d0b10]">
      <Sidebar />

      <main className="min-w-0 flex-1 overflow-y-auto">
        <div className="flex h-full flex-col items-center justify-center gap-2 p-8 text-center">
          <p className="text-2xl font-semibold text-[#e8e8ea]">Get started</p>
          <p className="max-w-sm text-sm text-[#8a8a94]">
            Click the panel icon in the sidebar header to collapse it to the icon
            rail, or the workspace name to open the organization menu.
          </p>
        </div>
      </main>
    </div>
  );
}
