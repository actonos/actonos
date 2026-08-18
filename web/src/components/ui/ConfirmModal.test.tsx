import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { ConfirmModal } from './ConfirmModal';

describe('ConfirmModal', () => {
  it('waits for async confirmation before closing', async () => {
    let resolve!: () => void;
    const pending = new Promise<void>((done) => {
      resolve = done;
    });
    const onClose = vi.fn();
    render(
      <ConfirmModal
        isOpen
        onClose={onClose}
        onConfirm={() => pending}
        title="Confirm action"
        description="Description"
      />
    );

    fireEvent.click(screen.getByRole('button', { name: 'Confirm' }));
    expect(onClose).not.toHaveBeenCalled();
    resolve();
    await vi.waitFor(() => expect(onClose).toHaveBeenCalledOnce());
  });
});
