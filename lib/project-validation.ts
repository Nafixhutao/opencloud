import { z } from 'zod';

export const createProjectSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, 'Enter a project name.')
    .max(100, 'Project names must be 100 characters or fewer.'),
});

export type CreateProjectValues = z.infer<typeof createProjectSchema>;
