import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';

import { Sidebar } from '@/planing-ui/sidebar/sidebar';

afterEach(cleanup);

describe('reference Sidebar', () => {
  it('renders the workspace identity and the account row', () => {
    render(<Sidebar />);

    expect(screen.getByText('nazxf')).toBeInTheDocument();
    expect(screen.getByText('Pro Plus')).toBeInTheDocument();
    expect(screen.getByText('Nazxf')).toBeInTheDocument();
    expect(screen.getByText('Admin')).toBeInTheDocument();
  });

  it('starts with Review expanded and every other section collapsed', () => {
    render(<Sidebar />);

    expect(screen.getByRole('button', { name: /Review/ })).toHaveAttribute(
      'aria-expanded',
      'true',
    );

    for (const label of ['Analytics', 'Slack', 'Discord', 'Security', 'Plan']) {
      expect(screen.getByRole('button', { name: new RegExp(label) })).toHaveAttribute(
        'aria-expanded',
        'false',
      );
    }
  });

  it('collapses and reopens Review when its trigger is activated', () => {
    render(<Sidebar />);
    const trigger = screen.getByRole('button', { name: /Review/ });

    fireEvent.click(trigger);
    expect(trigger).toHaveAttribute('aria-expanded', 'false');

    fireEvent.click(trigger);
    expect(trigger).toHaveAttribute('aria-expanded', 'true');
  });

  it('renders the Review submenu rows with their badges', () => {
    render(<Sidebar />);

    for (const label of [
      'Triage',
      'Repositories',
      'Integrations',
      'Learnings',
      'Caches',
      'Organization Settings',
    ]) {
      expect(screen.getByText(label)).toBeInTheDocument();
    }

    expect(screen.getByText('Beta')).toBeInTheDocument();
    // Slack and Security both carry a "New" badge.
    expect(screen.getAllByText('New')).toHaveLength(2);
  });

  it('marks both the active and the selected submenu row as current', () => {
    render(<Sidebar />);

    expect(screen.getByRole('button', { name: 'Integrations' })).toHaveAttribute(
      'aria-current',
      'page',
    );
    expect(screen.getByRole('button', { name: 'Repositories' })).toHaveAttribute(
      'aria-current',
      'page',
    );
    expect(screen.getByRole('button', { name: 'Caches' })).not.toHaveAttribute(
      'aria-current',
    );
  });

  it('reveals the overflow rows from the View more pill', () => {
    render(<Sidebar />);

    expect(screen.queryByText('Members')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: /View more/ }));

    expect(screen.getByText('Members')).toBeInTheDocument();
    expect(screen.getByText('Docs')).toBeInTheDocument();
    expect(screen.getByText('Support')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /View less/ })).toBeInTheDocument();
  });

  it('collapses to the icon rail and keeps rows reachable by name', () => {
    render(<Sidebar />);

    fireEvent.click(screen.getByRole('button', { name: 'Collapse sidebar' }));

    // Labels are visually hidden at rail width, so rows fall back to
    // aria-label. Submenus are dropped entirely.
    expect(screen.getByRole('button', { name: 'Review' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Expand sidebar' })).toBeInTheDocument();
    expect(screen.queryByText('Organization Settings')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Expand sidebar' }));
    expect(screen.getByText('Organization Settings')).toBeInTheDocument();
  });

  it('opens the workspace menu with its organization actions', () => {
    render(<Sidebar />);

    fireEvent.click(screen.getByRole('button', { name: /nazxf/ }));

    expect(screen.getByText('Refresh list')).toBeInTheDocument();
    expect(screen.getByText('Add organization')).toBeInTheDocument();
  });
});
