import { cn } from '@/lib/utils';

type WorkspaceAvatarProps = {
  size: number;
  className?: string;
};

/**
 * Stand-in for the profile photo used in the reference.
 *
 * The design shows a real portrait; with no asset to ship, this approximates
 * its warm tone with layered radial gradients plus the soft light ring that
 * separates it from the near-black surface.
 */
export function WorkspaceAvatar({ size, className }: WorkspaceAvatarProps) {
  return (
    <span
      aria-hidden="true"
      style={{ width: size, height: size }}
      className={cn(
        'relative shrink-0 overflow-hidden rounded-full',
        'ring-1 ring-white/15',
        className,
      )}
    >
      <span
        className="absolute inset-0"
        style={{
          backgroundImage: [
            'radial-gradient(circle at 50% 78%, #f0c9a8 0%, #f0c9a8 26%, transparent 27%)',
            'radial-gradient(circle at 50% 34%, #3a2a30 0%, #3a2a30 30%, transparent 31%)',
            'radial-gradient(circle at 30% 22%, #b8785a 0%, #8d5340 45%, transparent 46%)',
            'linear-gradient(160deg, #d99a72 0%, #b2705a 45%, #6d4a55 100%)',
          ].join(','),
        }}
      />
    </span>
  );
}
