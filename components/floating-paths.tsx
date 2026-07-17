export function FloatingPaths({ position }: { position: number }) {
  const paths = Array.from({ length: 36 }, (_, index) => ({
    id: index,
    d: `M-${380 - index * 5 * position} -${189 + index * 6}C-${
      380 - index * 5 * position
    } -${189 + index * 6} -${312 - index * 5 * position} ${216 - index * 6} ${
      152 - index * 5 * position
    } ${343 - index * 6}C${616 - index * 5 * position} ${470 - index * 6} ${
      684 - index * 5 * position
    } ${875 - index * 6} ${684 - index * 5 * position} ${875 - index * 6}`,
    width: 0.5 + index * 0.03,
  }));

  return (
    <div aria-hidden="true" className="pointer-events-none absolute inset-0">
      <svg
        className="auth-floating-paths h-full w-full text-foreground"
        fill="none"
        focusable="false"
        viewBox="0 0 696 316"
      >
        {paths.map((path) => (
          <path
            d={path.d}
            key={path.id}
            stroke="currentColor"
            strokeOpacity={0.04 + path.id * 0.008}
            strokeWidth={path.width}
          />
        ))}
      </svg>
    </div>
  );
}
