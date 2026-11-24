// A confirmation of a dangerous action (a reboot, a removal, stopping a service): a dialog over the page, answered
// with true or false. Nothing dangerous runs without it.
export interface Ask {
  title: string;
  text: string;
  ok: string;
  danger?: boolean;
  /** The label of the cancel button, when it is an answer of its own ("keep both"). */
  cancel?: string;
}

export const dialog = $state<{ open: boolean; ask: Ask | null; resolve: ((v: boolean) => void) | null }>({ open: false, ask: null, resolve: null });

export function confirmAsk(a: Ask): Promise<boolean> {
  return new Promise((resolve) => {
    dialog.ask = a;
    dialog.resolve = resolve;
    dialog.open = true;
  });
}

export function answer(v: boolean): void {
  dialog.open = false;
  dialog.resolve?.(v);
  dialog.resolve = null;
}

// A question with a text field (a name of a file, a folder): the answer is the text, or null when it is cancelled.
export interface PromptSpec {
  title: string;
  label: string;
  value?: string;
  ok: string;
  /** Returns an error text for a bad value, or '' when it is fine. */
  check?: (v: string) => string;
}

export const prompter = $state<{ open: boolean; spec: PromptSpec | null; resolve: ((v: string | null) => void) | null }>({ open: false, spec: null, resolve: null });

export function promptAsk(spec: PromptSpec): Promise<string | null> {
  return new Promise((resolve) => {
    prompter.spec = spec;
    prompter.resolve = resolve;
    prompter.open = true;
  });
}

export function promptAnswer(v: string | null): void {
  prompter.open = false;
  prompter.resolve?.(v);
  prompter.resolve = null;
}
