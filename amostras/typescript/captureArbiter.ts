// Amostra do Zone. Arquivo original: frontend/src/features/realtime/captureArbiter.ts
// O código completo fica em repositório privado. Todos os direitos reservados (ver LICENSE).

/**
 * E12 · Árbitro de captura por tipo de mídia, comum às chamadas do Zone e ao
 * escritório virtual incorporado. Copiado de
 * escritorio/shared/media/captureArbiter.ts (mesma API).
 * Microfone e tela têm titulares separados; nunca há preempção.
 */
export type CaptureKind = "microphone" | "screen";
export interface CaptureOwnership { acquire(owner: object): boolean; release(owner: object): void }
export interface CaptureHolder { owner: object; label: string }
type Listener = (kind: CaptureKind, holder: CaptureHolder | null) => void;

export class CaptureArbiter {
  private holders = new Map<CaptureKind, CaptureHolder>();
  private listeners = new Set<Listener>();

  acquire(kind: CaptureKind, owner: object, label: string): boolean {
    const current = this.holders.get(kind);
    if (current && current.owner !== owner) return false;
    if (!current) { this.holders.set(kind, { owner, label }); this.emit(kind); }
    return true;
  }

  release(kind: CaptureKind, owner: object) {
    if (this.holders.get(kind)?.owner !== owner) return;
    this.holders.delete(kind); this.emit(kind);
  }

  holder(kind: CaptureKind): CaptureHolder | null { return this.holders.get(kind) ?? null; }

  ownership(kind: CaptureKind, label: string): CaptureOwnership {
    return { acquire: (owner) => this.acquire(kind, owner, label), release: (owner) => this.release(kind, owner) };
  }

  subscribe(listener: Listener) { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; }

  private emit(kind: CaptureKind) {
    const holder = this.holder(kind);
    for (const listener of [...this.listeners]) {
      try { listener(kind, holder); } catch { /* Um ouvinte com erro não interrompe a captura. */ }
    }
  }
}

/** Uma instância por janela do aplicativo. */
export const zoneCaptureArbiter = new CaptureArbiter();
export const ZONE_CALL_LABEL = "Chamada do Zone";
export const OFFICE_LABEL = "Escritório virtual";
