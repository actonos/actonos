import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { Modal } from './Modal';

describe('Modal', () => {
  it('exposes dialog semantics and closes with Escape', async () => {
    const onClose = vi.fn();
    render(<Modal isOpen onClose={onClose} title="Test dialog"><button>Action</button></Modal>);
    expect(screen.getByRole('dialog', { name: 'Test dialog' })).toHaveAttribute('aria-modal', 'true');
    await userEvent.keyboard('{Escape}');
    expect(onClose).toHaveBeenCalledOnce();
  });
});
