import type { Metadata } from 'next';
import { redirect } from 'next/navigation';

import { SignOutButton } from '@/components/auth/sign-out-button';
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { getSession } from '@/lib/session';

// oxlint-disable-next-line react/only-export-components -- Next.js reads this page export.
export const metadata: Metadata = {
  title: 'Dashboard · Cevra',
};

export default async function DashboardPage() {
  const session = await getSession();
  if (!session) {
    redirect('/');
  }

  return (
    <main className="flex min-h-svh w-full items-center justify-center bg-background p-6">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>
            <h1>Dashboard</h1>
          </CardTitle>
          <CardDescription>You are logged in.</CardDescription>
        </CardHeader>
        <CardContent className="text-sm">
          <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1">
            <dt className="text-muted-foreground">Name</dt>
            <dd className="min-w-0 break-words">{session.user.name}</dd>
            <dt className="text-muted-foreground">Email</dt>
            <dd className="min-w-0 break-all">{session.user.email}</dd>
          </dl>
        </CardContent>
        <CardFooter>
          <SignOutButton />
        </CardFooter>
      </Card>
    </main>
  );
}
