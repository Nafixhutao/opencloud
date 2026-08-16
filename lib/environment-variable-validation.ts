import { z } from 'zod';

export const ENVIRONMENT_VARIABLE_ENVIRONMENTS = ['production', 'preview', 'development'] as const;

// Mirrors the Go service: ^[A-Z][A-Z0-9_]{0,127}$ plus non-empty value.
// Reserved-prefix rejection stays server-side so the list stays single-sourced.
export const createEnvironmentVariableSchema = z.object({
  key: z
    .string()
    .trim()
    .min(1, 'Enter a variable key.')
    .max(128, 'Keys must be 128 characters or fewer.')
    .transform((value) => value.toUpperCase())
    .refine(
      (value) => /^[A-Z][A-Z0-9_]{0,127}$/.test(value),
      'Keys start with an uppercase letter and use only A–Z, 0–9, and underscores.',
    ),
  value: z.string().min(1, 'Enter a value.'),
  is_secret: z.boolean(),
  environment: z.enum(ENVIRONMENT_VARIABLE_ENVIRONMENTS),
});

export type CreateEnvironmentVariableValues = z.infer<typeof createEnvironmentVariableSchema>;

export const updateEnvironmentVariableSchema = z.object({
  value: z.string().min(1, 'Enter a value.'),
});

export type UpdateEnvironmentVariableValues = z.infer<typeof updateEnvironmentVariableSchema>;
