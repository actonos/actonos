import { StrictMode } from 'react';
import ReactDOM from 'react-dom/client';
import { App } from './App';
import { i18nReady } from './lib/i18n';
import './index.css';

const root = ReactDOM.createRoot(document.getElementById('root')!);

function render() {
  root.render(
    <StrictMode>
      <App />
    </StrictMode>,
  );
}

// Translations load as separate chunks; wait for them so the first paint is
// already localized. If loading fails, render anyway and fall back to keys.
i18nReady.then(render, (error: unknown) => {
  console.error('Failed to load translations', error);
  render();
});
