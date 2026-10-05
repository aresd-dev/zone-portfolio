# Supressão de ruído com IA

Nas primeiras chamadas, dava para ouvir respiração, teclado e louça. A supressão
padrão do navegador (`noiseSuppression` do WebRTC) não resolvia. Antes de escolher
uma solução, medi as opções.

## Como medi

- **Sinal**: fala sintetizada em português (48 kHz, −22 dBFS) misturada a ruídos
  gerados: teclado mecânico, louça, respiração perto do microfone e ventilador.
- **Offline**: os mesmos worklets rodando num `OfflineAudioContext` do Edge, medindo
  o nível que sobra nas pausas e o SDR na fala (quanto maior, menos distorção).
- **Tempo real**: build de produção no Edge, com um microfone falso tocando o
  arquivo e o `getUserMedia` real.

## Comparação offline

| | Entrada | RNNoise | GTCRN | GTCRN + portão de voz |
| --- | --- | --- | --- | --- |
| Pausa com teclado | −40,9 dB | −46,9 | −61,0 | −59,9 |
| Pausa com respiração | −32,5 dB | −40,0 | −60,1 | −66,7 |
| Pausa com louça | −34,1 dB | −49,0 | −58,9 | −70,6 |
| Fala limpa (SDR) | 30,0 | 12,5 | 27,8 | 27,8 |
| Fala + louça (SDR) | 14,8 | 10,9 | 25,7 | 25,3 |

O RNNoise foi descartado: reduz pouco o teclado e distorce a voz. O
[GTCRN](https://github.com/Xiaobin-Rong/gtcrn) (rede neural leve de realce de fala,
licença MIT) reduziu de 20 a 28 dB nas pausas e preservou a fala.

## Em tempo real

| Pausa | Microfone | Máxima (GTCRN + portão) | Forte (só GTCRN) |
| --- | --- | --- | --- |
| Teclado | −41 dB | −58 dB | −61 dB |
| Respiração | −33 dB | −91 dB | −74 dB |
| Louça | −35 dB | −60 dB | −65 dB |
| Ventilador | −54 dB | −84 dB | −81 dB |

O nível da fala ficou igual antes e depois (−22,4 dB). Custo: cerca de 36 ms de
latência e 4% de um núcleo de CPU de quem fala.

## Implementação

```text
microfone → AudioContext 48 kHz → GTCRN (AudioWorklet + WebAssembly)
          → portão de voz (AudioWorklet próprio) → faixa enviada pelo WebRTC
```

- O modelo roda via [@sapphi-red/web-noise-suppressor](https://github.com/sapphi-red/web-noise-suppressor)
  (MIT), com versão fixa.
- O **portão de voz** é meu: envoltória rápida, piso de ruído adaptativo, limiar
  de máx(−48 dBFS, piso + 9 dB), retenção de 260 ms e atenuação de −36 dB em vez de
  corte seco. Uma antecipação de 4 ms abre o portão antes de cada palavra.
- Quatro níveis na chamada: Máxima, Forte, Padrão do navegador e Desligada. Trocar de
  nível substitui a faixa enviada sem reabrir o microfone nem renegociar a conexão.
- Sem AudioWorklet ou WebAssembly, ou se o modelo falhar, a chamada continua com a
  supressão do navegador e mostra um aviso.
- O escritório virtual usa cópias idênticas dos mesmos arquivos. No módulo
  incorporável, os worklets são carregados via `blob:`, compatível com o app Windows.

Código: [noiseSuppression.ts](../amostras/typescript/noiseSuppression.ts) e
[voiceGate.worklet.js](../amostras/typescript/voiceGate.worklet.js).

## Limites

- A medição usa fala sintetizada e ruídos gerados; microfones e ambientes reais variam.
- O ruído durante a fala é reduzido, não eliminado; o portão atua entre as falas.
