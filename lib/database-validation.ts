import { z } from 'zod';

export const databaseEngineSchema = z.enum(['postgres', 'mariadb']);

export const createDatabaseSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, 'Enter a database name.')
    .max(63, 'Database name must be 63 characters or fewer.')
    .transform((value) => value.toLowerCase())
    .refine(
      (value) => /^[a-z][a-z0-9_-]*$/.test(value),
      'Start with a letter and use only lowercase letters, numbers, underscores, or hyphens.',
    ),
  engine: databaseEngineSchema,
});

export type CreateDatabaseValues = z.infer<typeof createDatabaseSchema>;
