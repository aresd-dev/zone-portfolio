// Amostra do Zone. Arquivo original: frontend/src/features/realtime/voiceGate.worklet.js
// O código completo fica em repositório privado. Todos os direitos reservados (ver LICENSE).

/**
 * Zone · portão de voz suave (AudioWorklet, sem dependências).
 *
 * Fica depois do supressor neural. Entre as falas, o que sobra (respiração,
 * teclado, pratos já atenuados) costuma ficar abaixo do limiar e é reduzido em
 * vez de cortado de forma seca. Um atraso de 10 ms permite abrir o portão antes
 * do início de cada palavra, e a liberação lenta evita "picotar" finais de frase.
 */
const dbToGain = (db) => 10 ** (db / 20);

class VoiceGateProcessor extends AudioWorkletProcessor {
  constructor(options) {
    super();
    const o = options?.processorOptions ?? {};
    this.enabled = o.enabled !== false;
    this.thresholdDb = o.thresholdDb ?? -48;      // abaixo disto, nunca abre
    this.marginDb = o.marginDb ?? 9;              // acima do ruído residual medido
    this.floorGain = dbToGain(o.attenuationDb ?? -36);
    this.holdSamples = Math.round(((o.holdMs ?? 260) / 1000) * sampleRate);
    this.attack = 1 - Math.exp(-1 / ((o.attackMs ?? 3) / 1000 * sampleRate));
    this.release = 1 - Math.exp(-1 / ((o.releaseMs ?? 140) / 1000 * sampleRate));
    this.delay = new Float32Array(Math.max(1, Math.round(((o.lookaheadMs ?? 10) / 1000) * sampleRate)));
    this.write = 0;
    this.level = -100;   // envoltória rápida em dB
    this.floor = -70;    // estimativa lenta do ruído residual
    this.hold = 0;
    this.gain = this.floorGain;
    // Sobe o piso devagar (6 dB/s) e desce rápido: segue o ambiente sem
    // confundir uma fala longa com ruído.
    this.floorRise = 6 * 128 / sampleRate;
    this.port.onmessage = (event) => {
      if (event.data && typeof event.data.enabled === "boolean") this.enabled = event.data.enabled;
    };
  }

  process(inputs, outputs) {
    const input = inputs[0]?.[0];
    const output = outputs[0]?.[0];
    if (!output) return true;
    if (!input) { output.fill(0); return true; }
    let energy = 0;
    for (let i = 0; i < input.length; i++) energy += input[i] * input[i];
    const db = 10 * Math.log10(energy / input.length + 1e-12);
    this.level = db > this.level ? db : this.level + (db - this.level) * 0.25;
    if (this.level < this.floor) this.floor = this.level;
    else this.floor = Math.min(this.floor + this.floorRise, -20);
    const threshold = Math.max(this.thresholdDb, this.floor + this.marginDb);
    if (this.level > threshold) this.hold = this.holdSamples;
    else this.hold = Math.max(0, this.hold - input.length);
    const target = !this.enabled || this.hold > 0 ? 1 : this.floorGain;
    const size = this.delay.length;
    for (let i = 0; i < input.length; i++) {
      const delayed = this.delay[this.write];
      this.delay[this.write] = input[i];
      this.write = (this.write + 1) % size;
      this.gain += (target - this.gain) * (target > this.gain ? this.attack : this.release);
      output[i] = delayed * this.gain;
    }
    for (let channel = 1; channel < outputs[0].length; channel++) outputs[0][channel].set(output);
    return true;
  }
}

registerProcessor("zone-voice-gate", VoiceGateProcessor);
