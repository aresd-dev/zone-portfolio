# Supressão de ruído com IA

Nas primeiras chamadas, dava para ouvir respiração, teclado e louça. A supressão
padrão do navegador (`noiseSuppression` do WebRTC) não resolvia. Antes de escolher
uma solução, medi as opções.

## Níveis

A pessoa escolhe o nível ao lado de "Voz conectada", em qualquer tela da chamada:

| Nível | O que faz |
| --- | --- |
| Máxima (padrão) | DeepFilterNet3 compilado para WebAssembly. Enquanto o motor baixa (só na primeira vez) ou se o navegador não puder rodá-lo, usa o GTCRN com o portão de voz |
| Forte | GTCRN com o portão de voz: usa menos CPU |
| Padrão do navegador | A supressão do próprio navegador |
| Desligada | Sem supressão |

Trocar de nível não reabre o microfone nem renegocia a chamada: entre os níveis com IA
o motor muda dentro do mesmo grafo de áudio, e para os outros a faixa enviada é
substituída (`replaceTrack`).

## Como medi

- **Sinal**: fala sintetizada em português (48 kHz, −22 dBFS) misturada a ruídos
  gerados: teclado mecânico, louça, respiração perto do microfone e ventilador.
- **Offline**: os mesmos worklets rodando num `OfflineAudioContext` do Edge, medindo
  o nível que sobra nas pausas e o SDR na fala (quanto maior, menos distorção).
- **Tempo real**: build de produção no Edge, com um microfone falso tocando o
  arquivo e o `getUserMedia` real.

## Primeira rodada: RNNoise e GTCRN (04/10/2026)

**Offline:**

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

**Em tempo real:**

| Pausa | Microfone | GTCRN + portão | Só GTCRN |
| --- | --- | --- | --- |
| Teclado | −41 dB | −58 dB | −61 dB |
| Respiração | −33 dB | −91 dB | −74 dB |
| Louça | −35 dB | −60 dB | −65 dB |
| Ventilador | −54 dB | −84 dB | −81 dB |

O nível da fala ficou igual antes e depois (−22,4 dB). Custo: cerca de 36 ms de
latência e 4% de um núcleo de CPU de quem fala.

## DeepFilterNet3 na Máxima (06/10/2026)

Batidas de louça ainda passavam. O [DeepFilterNet3](https://github.com/Rikorose/DeepFilterNet)
(MIT ou Apache-2.0) é melhor com ruídos impulsivos, mas não tem build oficial para
navegador, e os pacotes prontos baixam o motor de um CDN de terceiros, o que o app
Windows não permite (ele só carrega o endereço do Zone).

- Compilei o motor do código oficial (Rust) para WebAssembly com SIMD, com um script
  que reproduz o build a partir de um commit fixo e confere os checksums.
- Motor (10,7 MB) e modelo (8 MB) são servidos pelo próprio Zone, com cache de um ano:
  cada pessoa baixa uma vez.
- Roda num AudioWorklet. O navegador entrega blocos de 128 amostras e o modelo processa
  quadros de 480 (10 ms); uma fila com aquecimento fixo garante que nunca falte
  amostra, sem estalos.
- Com o filtro passa-alta que o navegador aplica ao microfone, o modelo abaixava a voz
  em até ~6 dB. O worklet mede essa queda só nos quadros de fala e devolve o volume,
  com teto de +8 dB, para não trazer o ruído de volta.
- Custo: ~1,6 ms por quadro de 10 ms e ~40 ms de latência. Navegadores sem SIMD usam o GTCRN.

Em tempo real (Edge, microfone falso com o tratamento do navegador ligado), comparado
ao GTCRN + portão na versão da mesma data:

| Pausa | GTCRN + portão | DeepFilterNet3 |
| --- | --- | --- |
| Teclado | −65 dB | −70 dB |
| Respiração | −90 dB | −81 dB |
| Louça | −62 dB | silêncio (−120 dB) |
| Ventilador | −81 dB | silêncio (−120 dB) |

O nível da fala ficou preservado e a inteligibilidade (ESTOI) praticamente igual à do
GTCRN: 0,961 contra 0,965 na fala limpa e 0,891 contra 0,870 com ruído misto. Um
portão de voz depois do DeepFilterNet3 reduzia a inteligibilidade sem ganho audível,
então a Máxima não usa o portão.

## Implementação

```text
Máxima: microfone → AudioContext 48 kHz → DeepFilterNet3 (AudioWorklet + WebAssembly SIMD)
                  → faixa enviada pelo WebRTC
Forte:  microfone → AudioContext 48 kHz → GTCRN (AudioWorklet + WebAssembly)
                  → portão de voz (AudioWorklet próprio) → faixa enviada pelo WebRTC
```

- O GTCRN roda via [@sapphi-red/web-noise-suppressor](https://github.com/sapphi-red/web-noise-suppressor)
  (MIT), com versão fixa.
- O **portão de voz** é meu: envoltória rápida, piso de ruído adaptativo, limiar
  de máx(−48 dBFS, piso + 9 dB), retenção de 260 ms e atenuação de −36 dB em vez de
  corte seco. Ele também recebe o microfone antes do modelo: um trecho de que o modelo
  removeu mais de 15 dB (tecla, louça) não conta como voz, e o portão só abre quando ao
  menos 3 dos últimos 6 blocos parecem voz, então um estalo isolado não o abre.
- Sem AudioWorklet ou WebAssembly, ou se o modelo falhar, a chamada continua com a
  supressão do navegador e mostra um aviso.
- O escritório virtual usa cópias idênticas dos mesmos arquivos. No módulo
  incorporável, os worklets são carregados via `blob:`, compatível com o app Windows.

Código (versão de 05/10, antes do DeepFilterNet3):
[noiseSuppression.ts](../amostras/typescript/noiseSuppression.ts) e
[voiceGate.worklet.js](../amostras/typescript/voiceGate.worklet.js).

## Limites

- A medição usa fala sintetizada e ruídos gerados; microfones e ambientes reais variam.
  Se a voz soar "metálica" em algum aparelho, os níveis Forte e Padrão do navegador
  continuam disponíveis.
- O ruído durante a fala é reduzido, não eliminado; a maior redução acontece entre as falas.
