// Amostra do Zone. Arquivo original: frontend/src/features/realtime/noiseSuppression.ts
// O código completo fica em repositório privado. Todos os direitos reservados (ver LICENSE).

/**
 * Supressão de ruído do microfone.
 *
 * "maxima" e "forte" usam o GTCRN (rede neural leve de realce de fala, MIT) via
 * @sapphi-red/web-noise-suppressor, num AudioWorklet. Em teste com fala
 * sintetizada e ruídos gerados, reduziu teclado, respiração e pratos de 20 a
 * 28 dB nas pausas, mantendo a voz (ver docs/SUPRESSAO-RUIDO.md). "maxima" soma
 * o portão de voz suave, que atenua o que sobra entre as falas.
 */
import gtcrnWorkletUrl from "@sapphi-red/web-noise-suppressor/gtcrnWorklet.js?url";
import gtcrnWasmUrl from "@sapphi-red/web-noise-suppressor/gtcrn.wasm?url";
import voiceGateUrl from "./voiceGate.worklet.js?url";

export type NoiseLevel = "maxima" | "forte" | "navegador" | "desligada";
export const NOISE_LEVELS: readonly NoiseLevel[] = ["maxima", "forte", "navegador", "desligada"];
export const NOISE_LABELS: Record<NoiseLevel, string> = {
  maxima: "Máxima — IA + corta respiração e ruído entre falas",
  forte: "Forte — IA, som mais natural entre falas",
  navegador: "Padrão do navegador",
  desligada: "Desligada",
};
export const isNoiseLevel = (value: unknown): value is NoiseLevel => NOISE_LEVELS.includes(value as NoiseLevel);
export const usesModel = (level: NoiseLevel) => level === "maxima" || level === "forte";

/** AudioWorklet + WebAssembly; sem isso, o navegador assume. */
export function advancedNoiseSupported(): boolean {
  return typeof AudioContext === "function" && typeof AudioWorkletNode === "function"
    && typeof WebAssembly === "object" && "audioWorklet" in AudioContext.prototype;
}

export interface NoiseProcessor {
  /** Faixa processada que vai para as conexões WebRTC. */
  readonly track: MediaStreamTrack;
  readonly stream: MediaStream;
  setLevel(level: "maxima" | "forte"): void;
  close(): void;
}
export type NoiseProcessorFactory = (microphone: MediaStream, level: "maxima" | "forte") => Promise<NoiseProcessor>;

// The model binary is fetched once per page and copied into each worklet.
let modelBinary: Promise<ArrayBuffer> | undefined;
function loadModel(): Promise<ArrayBuffer> {
  modelBinary ??= fetch(gtcrnWasmUrl, { credentials: "same-origin" }).then((response) => {
    if (!response.ok) throw new Error("noise_model");
    return response.arrayBuffer();
  }).catch((error) => { modelBinary = undefined; throw error; });
  return modelBinary;
}

// Library builds (the office's embeddable module) inline assets as data: URLs.
// Worklets load more reliably from blob: URLs, which the Windows client allows.
async function workletURL(url: string): Promise<{ url: string; release(): void }> {
  if (!url.startsWith("data:")) return { url, release() {} };
  const source = await (await fetch(url)).blob();
  const objectURL = URL.createObjectURL(new Blob([source], { type: "text/javascript" }));
  return { url: objectURL, release: () => URL.revokeObjectURL(objectURL) };
}

const GATE = { thresholdDb: -48, marginDb: 9, attenuationDb: -36, holdMs: 260, attackMs: 3, releaseMs: 140, lookaheadMs: 4 };

export const createNoiseProcessor: NoiseProcessorFactory = async (microphone, level) => {
  const [{ GtcrnWorkletNode }, wasmBinary] = await Promise.all([import("@sapphi-red/web-noise-suppressor"), loadModel()]);
  const context = new AudioContext({ sampleRate: 48_000, latencyHint: "interactive" });
  let model: InstanceType<typeof GtcrnWorkletNode> | undefined;
  try {
    const modules = await Promise.all([workletURL(gtcrnWorkletUrl), workletURL(voiceGateUrl)]);
    try { await Promise.all(modules.map((module) => context.audioWorklet.addModule(module.url))); }
    finally { modules.forEach((module) => module.release()); }
    // A suspended context would send silence: require it to run, or fall back.
    const running = () => context.state === "running";
    if (!running()) {
      await Promise.race([context.resume(), new Promise((resolve) => setTimeout(resolve, 1_500))]);
      if (!running()) throw new Error("audio_context_suspended");
    }
    const source = context.createMediaStreamSource(microphone);
    model = new GtcrnWorkletNode(context, { maxChannels: 1, wasmBinary });
    model.channelCount = 1; model.channelCountMode = "explicit";
    const gate = new AudioWorkletNode(context, "zone-voice-gate", {
      channelCount: 1, channelCountMode: "explicit", outputChannelCount: [1],
      processorOptions: { ...GATE, enabled: level === "maxima" },
    });
    const destination = context.createMediaStreamDestination();
    destination.channelCount = 1; destination.channelCountMode = "explicit";
    source.connect(model); model.connect(gate); gate.connect(destination);
    const track = destination.stream.getAudioTracks()[0];
    if (!track) throw new Error("noise_track");
    // Device changes may suspend the context; resume instead of going silent.
    context.onstatechange = () => { if (context.state === "suspended") void context.resume().catch(() => {}); };
    let closed = false;
    const active = model;
    return {
      track, stream: destination.stream,
      setLevel(next) { if (!closed) gate.port.postMessage({ enabled: next === "maxima" }); },
      close() {
        if (closed) return;
        closed = true;
        context.onstatechange = null;
        try { source.disconnect(); active.disconnect(); gate.disconnect(); } catch { /* already disconnected */ }
        active.destroy();
        track.stop();
        void context.close().catch(() => {});
      },
    };
  } catch (error) {
    model?.destroy();
    void context.close().catch(() => {});
    throw error;
  }
};
