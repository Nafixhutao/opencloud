import { describe, expect, it } from 'vitest';

import { safeInternalPath } from '@/lib/safe-redirect';

describe('safeInternalPath', () => {
  it('keeps same-origin paths and query strings', () => {
    expect(safeInternalPath('/sites/site-id?page=2')).toBe('/sites/site-id?page=2');
  });

  it.each([
    'https://evil.example/',
    'javascript:alert(1)',
    '//evil.example/',
    '/\\evil.example/',
    '/%5cevil.example/',
    '/%255cevil.example/',
    '/%2f%2fevil.example/',
    '/%252f%252fevil.example/',
    '/%00dashboard',
    '%E0%A4%A',
  ])('rejects unsafe callback %s', (value) => {
    expect(safeInternalPath(value)).toBe('/dashboard');
  });
});
