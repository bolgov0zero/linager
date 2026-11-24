// A short message at the bottom of the window (the `.toast` of the style sheet).
let el: HTMLDivElement | null = null;
let timer: ReturnType<typeof setTimeout> | undefined;

export function showToast(text: string): void {
  if (!el) {
    el = document.createElement('div');
    el.className = 'toast';
    el.setAttribute('role', 'status');
    document.body.appendChild(el);
  }
  el.textContent = text;
  // a frame later, so that the transition starts
  requestAnimationFrame(() => el?.classList.add('on'));
  clearTimeout(timer);
  timer = setTimeout(() => el?.classList.remove('on'), 3500);
}
