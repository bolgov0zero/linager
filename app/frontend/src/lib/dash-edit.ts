import type { Group } from './api';

/** The edit mode of the dashboard: the rows are in the order of the server (no sorting), can be dragged and have the controls of a card. */
export interface RowEdit {
  groups: Group[];
  move: (id: string, dir: -1 | 1) => void;
  group: (id: string, value: string) => void;
  remove: (id: string) => void;
  del: (id: string) => void;
  dragStart: (e: DragEvent, id: string) => void;
  dragOver: (e: DragEvent, id: string) => void;
  drop: (e: DragEvent, id: string) => void;
  dragEnd: () => void;
  overId: () => string | null;
  dragId: () => string | null;
}
