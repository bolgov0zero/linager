import { mount } from 'svelte';
import './app.css';
import { loadPrefs } from './lib/prefs';
import { initTheme } from './lib/theme';

// no browser menu (reload, inspect, ...) except where text is copied or pasted
document.addEventListener('contextmenu', (e) => {
  if (!(e.target instanceof Element) || !e.target.closest('input, textarea, .term-host')) e.preventDefault();
});

// the saved choices are read first: the theme and the language are decided when the interface is built
void (async () => {
  await loadPrefs();
  initTheme();
  const { default: App } = await import('./App.svelte');
  mount(App, { target: document.getElementById('app')! });
})();
