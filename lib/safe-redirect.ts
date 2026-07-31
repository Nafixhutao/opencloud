const redirectBase = 'https://opencloud.invalid';

// safeInternalPath accepts only a same-origin path. It rejects browser
// backslash normalization and encoded separator tricks before auth or Next.js
// receives the callback.
export function safeInternalPath(
  value: string | undefined,
  fallback = '/dashboard',
): string {
  if (!value || !value.startsWith('/')) {
    return fallback;
  }
  let decoded = value;
  for (let pass = 0; pass < 3; pass++) {
    try {
      const next = decodeURIComponent(decoded);
      if (next === decoded) {
        break;
      }
      decoded = next;
    } catch {
      return fallback;
    }
  }
  if (
    decoded.startsWith('//') ||
    decoded.includes('\\') ||
    hasControlCharacter(decoded)
  ) {
    return fallback;
  }
  try {
    const resolved = new URL(value, redirectBase);
    if (resolved.origin !== redirectBase) {
      return fallback;
    }
    return `${resolved.pathname}${resolved.search}${resolved.hash}`;
  } catch {
    return fallback;
  }
}

function hasControlCharacter(value: string): boolean {
  for (const character of value) {
    const codePoint = character.codePointAt(0) ?? 0;
    if (codePoint <= 31 || codePoint === 127) {
      return true;
    }
  }
  return false;
}
