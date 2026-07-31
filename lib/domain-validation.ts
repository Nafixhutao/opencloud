import { z } from 'zod';

export const attachDomainSchema = z.object({
  hostname: z
    .string()
    .trim()
    .min(1, 'Enter a hostname you control.')
    .max(253, 'Hostname must be 253 characters or fewer.')
    .refine(
      (value) =>
        value.includes('.') &&
        !value.includes('://') &&
        !/[\s/:*]/u.test(value) &&
        !value.startsWith('.') &&
        !value.endsWith('.'),
      'Enter a hostname only, for example www.example.com.',
    ),
});

export type AttachDomainValues = z.infer<typeof attachDomainSchema>;
