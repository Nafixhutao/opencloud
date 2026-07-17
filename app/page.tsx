import Link from 'next/link';

export default function Home() {
  return (
    <main className="flex min-h-svh flex-col items-center justify-center gap-4 p-6">
      <h1 className="text-2xl font-medium">OpenCloud</h1>
      <p className="text-sm text-muted-foreground">
        The OpenCloud control plane is under active development.
      </p>
      <nav className="flex gap-4 text-sm">
        <Link href="/login" className="underline underline-offset-4">
          Log in
        </Link>
        <Link href="/register" className="underline underline-offset-4">
          Register
        </Link>
      </nav>
    </main>
  );
}
