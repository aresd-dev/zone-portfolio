# Amostras do código

Arquivos reais do Zone, copiados sem alteração; só ganharam um cabeçalho com o caminho
original. Eles referenciam módulos que não estão aqui, porque o código completo é
privado. Para avaliação técnica, posso dar acesso de leitura ao repositório completo.

## Escritório virtual (Go)

**[world.go](go/escritorio/world.go)**: regras de movimento, independentes de HTTP.
O cliente envia só uma direção e um número de sequência; o servidor recusa pedidos
fora de ordem, rápidos demais (150 ms por passo) ou contra paredes, e a sequência é
consumida mesmo quando o passo é bloqueado, para que tentativas antigas não andem.
A chegada usa busca em largura para achar o piso livre mais próximo.

**[policy.go](go/escritorio/policy.go)**: decide, para cada par de pessoas, se há
texto, áudio e com qual volume. Considera distância euclidiana, salas de reunião,
áreas de foco, conversas trancadas com consentimento dos dois lados e linha de visão:
um traçado em grade que checa as duas células laterais nas diagonais, para que o som
não passe por frestas entre paredes que se tocam.

**[policy_test.go](go/escritorio/policy_test.go)**: testes em tabela que cobrem a
matriz de contextos e conferem que toda decisão é simétrica (A→B igual a B→A).

## Zone Call (Go)

**[jam.go](go/zone-call/jam.go)**: o Spotify não tem API pública de Jam, então a
chamada compartilha só o convite. A URL passa por validação estrita (HTTPS, hosts e
caminhos permitidos, sem credenciais, porta ou fragmento), há limite de 6 trocas a
cada 30 s por conexão, e o Jam some quando a chamada esvazia.

## Zone Call (Python)

**[sessions_repo.py](python/sessions_repo.py)**: refresh token opaco e rotativo. Só o
hash vai para o banco; a troca acontece numa transação com `SELECT … FOR UPDATE`;
duas abas concorrentes recebem o mesmo sucessor durante 30 s; reutilizar um token já
consumido depois disso revoga a sessão inteira. Um token aleatório com o mesmo ID
público não consegue revogar a sessão de ninguém.

**[moderation_repo.py](python/moderation_repo.py)**: papéis de administrador e
moderação da chamada. A autorização é conferida no banco a cada ação, edições
concorrentes são serializadas com `pg_advisory_xact_lock` e o administrador
principal vem só da configuração privada.

## Navegador (TypeScript / JavaScript)

**[noiseSuppression.ts](typescript/noiseSuppression.ts)**: pipeline de áudio com
AudioWorklet + WebAssembly (modelo GTCRN), carregamento único do modelo, fallback
quando o navegador não suporta e limpeza completa dos recursos.

**[voiceGate.worklet.js](typescript/voiceGate.worklet.js)**: portão de voz próprio,
sem dependências: envoltória em dB, piso de ruído adaptativo (sobe devagar, desce
rápido), retenção, ataque e liberação exponenciais e uma pequena antecipação para não
cortar o início das palavras. Medições em [supressao-de-ruido.md](../docs/supressao-de-ruido.md).

**[captureArbiter.ts](typescript/captureArbiter.ts)**: microfone e tela são
disputados pela chamada e pelo escritório na mesma janela. O árbitro tem um titular
por tipo de mídia, nunca faz preempção e isola erros de quem está ouvindo.
