import { NotFoundStacked } from '@/components/motion/not-found/stacked';
import { getSession } from '@/lib/session';

// Session-aware CTAs: an anonymous visitor is sent to sign in (the dashboard
// would only bounce them back to /login), while a signed-in customer returns
// to the dashboard.
export default async function NotFound() {
  const session = await getSession();
  const signedIn = Boolean(session);

  return (
    <main className="flex min-h-svh items-center justify-center px-4 py-16">
      <NotFoundStacked
        homeHref={signedIn ? '/dashboard' : '/login'}
        homeLabel={signedIn ? 'Back to dashboard' : 'Sign in'}
        browseHref={signedIn ? '/sites' : '/register'}
        browseLabel={signedIn ? 'View your sites' : 'Create account'}
      />
    </main>
  );
}
