import Image from 'next/image';

export default function Home() {
  return (
    <main className="flex min-h-svh flex-col items-center justify-center gap-4 p-6">
      <h1 className="sr-only">Cevra</h1>
      <Image
        src="/brand/cevra-logo.png"
        alt="Cevra"
        width={198}
        height={60}
        priority
        className="h-10 w-auto"
      />
      <p className="text-sm text-muted-foreground">
        The Cevra control plane is under active development.
      </p>
    </main>
  );
}
