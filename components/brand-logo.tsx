import Image from 'next/image';

import { cn } from '@/lib/utils';

type BrandLogoProps = {
  className?: string;
  priority?: boolean;
};

export function BrandLogo({ className, priority = false }: BrandLogoProps) {
  return (
    <Image
      src="/brand/cevra-logo.png"
      alt="Cevra"
      width={136}
      height={40}
      priority={priority}
      className={cn('h-7 w-auto', className)}
    />
  );
}
