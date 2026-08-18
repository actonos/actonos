import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';
import { DensityProvider, useDensity } from './DensityProvider';

function Probe() {
  const { density, toggleDensity } = useDensity();
  return <button onClick={toggleDensity}>{density}</button>;
}

describe('DensityProvider', () => {
  afterEach(() => {
    localStorage.clear();
    delete document.documentElement.dataset.density;
  });

  it('uses comfortable density by default and persists compact mode', async () => {
    const user = userEvent.setup();
    render(<DensityProvider><Probe /></DensityProvider>);

    expect(screen.getByRole('button')).toHaveTextContent('comfortable');
    await user.click(screen.getByRole('button'));

    expect(screen.getByRole('button')).toHaveTextContent('compact');
    expect(localStorage.getItem('actonos_ui_density')).toBe('compact');
    expect(document.documentElement.dataset.density).toBe('compact');
  });
});
