import type { NetChange, NetInterface } from './api';

/** Editable form for one interface. `mtu` is null while the input is empty. */
export interface NetForm {
  iface: string;
  mode: 'static' | 'dhcp';
  address: string;
  gateway: string;
  mtu: number | null;
  state: 'UP' | 'DOWN';
}

export function formFromIface(iface: NetInterface, existing?: NetChange): NetForm {
  if (existing) {
    return {
      iface: iface.name,
      mode: existing.mode,
      address: existing.address,
      gateway: existing.gateway,
      mtu: existing.mtu,
      state: existing.up ? 'UP' : 'DOWN',
    };
  }
  return {
    iface: iface.name,
    mode: iface.mode === 'dhcp' ? 'dhcp' : 'static',
    address: iface.addresses[0] ?? '',
    gateway: iface.gateway,
    mtu: iface.mtu,
    state: iface.state,
  };
}

/** Address and gateway are meaningless under DHCP, so they are sent empty. */
export function changeFromForm(form: NetForm): NetChange {
  const dhcp = form.mode === 'dhcp';
  return {
    iface: form.iface,
    mode: form.mode,
    address: dhcp ? '' : form.address.trim(),
    gateway: dhcp ? '' : form.gateway.trim(),
    mtu: Number(form.mtu ?? 0),
    up: form.state === 'UP',
  };
}

/** The current live configuration of an interface, in draft shape (for "was" comparisons). */
export function ifaceAsChange(iface: NetInterface): NetChange {
  return {
    iface: iface.name,
    mode: iface.mode === 'dhcp' ? 'dhcp' : 'static',
    address: iface.addresses[0] ?? '',
    gateway: iface.gateway,
    mtu: iface.mtu,
    up: iface.state === 'UP',
  };
}

/** Replaces the change for the same interface, or appends it. */
export function withChange(changes: NetChange[], change: NetChange): NetChange[] {
  return [...changes.filter((c) => c.iface !== change.iface), change];
}
