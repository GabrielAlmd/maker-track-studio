import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import Modal from './Modal';

it('keeps keyboard focus in the dialog and returns it to the opening control', async () => {
  const user = userEvent.setup();
  function Harness() {
    const [open, setOpen] = useState(false);
    return <>
      <button onClick={() => setOpen(true)}>Open form</button>
      {open && <Modal title="Add item" onClose={() => setOpen(false)}>
        <label>Name<input aria-label="Name" /></label>
        <button>Save</button>
      </Modal>}
    </>;
  }

  await user.click(render(<Harness />).getByText('Open form'));
  expect(screen.getByRole('textbox', { name: 'Name' })).toHaveFocus();
  await user.tab({ shift: true });
  expect(screen.getByRole('button', { name: 'Close' })).toHaveFocus();
  await user.tab({ shift: true });
  expect(screen.getByRole('button', { name: 'Save' })).toHaveFocus();
  await user.keyboard('{Escape}');
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Open form' })).toHaveFocus();
});
