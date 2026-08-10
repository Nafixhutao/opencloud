import { z } from 'zod';

export const createBucketSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, 'Enter a bucket name.')
    .max(63, 'Bucket name must be 63 characters or fewer.')
    .transform((value) => value.toLowerCase())
    .refine(
      (value) => /^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$/.test(value),
      'Use lowercase letters, numbers, and hyphens. Cannot start or end with a hyphen.',
    ),
});

export type CreateBucketValues = z.infer<typeof createBucketSchema>;
