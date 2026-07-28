import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { apiJSON } from '@/lib/api';
import { getSession } from '@/lib/session';

import DashboardPage from './page';

vi.mock('@/lib/api', () => ({
  apiJSON: vi.fn(),
}));

vi.mock('@/lib/session', () => ({
  getSession: vi.fn(),
}));

vi.mock('next/navigation', () => ({
  redirect: vi.fn(),
}));

const apiJSONMock = vi.mocked(apiJSON);
const getSessionMock = vi.mocked(getSession);

describe('DashboardPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    getSessionMock.mockResolvedValue({
      user: {
        id: 'dashboard-user',
        name: 'Ada Lovelace',
        email: 'ada@example.test',
        emailVerified: true,
        createdAt: new Date('2026-01-02T00:00:00Z'),
        updatedAt: new Date('2026-01-02T00:00:00Z'),
      },
      session: {
        id: 'dashboard-session',
        userId: 'dashboard-user',
        token: 'test-session-token',
        expiresAt: new Date('2027-01-02T00:00:00Z'),
        createdAt: new Date('2026-01-02T00:00:00Z'),
        updatedAt: new Date('2026-01-02T00:00:00Z'),
      },
    });
  });

  it('renders tenant aggregates beyond the first resource page', async () => {
    apiJSONMock.mockResolvedValue({
      data: {
        sites_total: 126,
        sites_active: 101,
        databases_total: 43,
        databases_active: 37,
      },
    });

    render(await DashboardPage());

    expect(apiJSONMock).toHaveBeenCalledTimes(1);
    expect(apiJSONMock).toHaveBeenCalledWith('/api/v1/overview');
    expect(screen.getByText('101')).toBeInTheDocument();
    expect(screen.getByText('126 total workloads')).toBeInTheDocument();
    expect(screen.getByText('43')).toBeInTheDocument();
    expect(screen.getByText('37 active instances')).toBeInTheDocument();
    expect(screen.getByText('101 active sites in this workspace.')).toBeInTheDocument();
  });
});
