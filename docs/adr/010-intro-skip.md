# ADR 010 — Pular abertura e créditos

- **Status:** aceito
- **Data:** 01/11/2026

## Contexto

A Sprint 9 traz o "Pular abertura" e o próximo episódio já nos créditos, como nos serviços de streaming. Não há metadados externos (AGENTS.md §1), então a posição da abertura precisa sair do próprio áudio.

## Decisões

1. **O fingerprint vem do próprio FFmpeg:** o muxer `chromaprint` (`-f chromaprint -fp_format raw`) gera um inteiro de 32 bits a cada ~0,124 s. Não entra dependência nova.
   - **Início:** os primeiros 10 min, no máximo 30% do episódio.
   - **Fim:** os últimos 4 min.
2. **A abertura é o áudio que os episódios têm em comum** (a mesma ideia do plugin *intro-skipper* do Jellyfin):
   - cada episódio é comparado com os outros da temporada; se for o único da temporada, com os da série;
   - um índice invertido dos pontos idênticos aponta os alinhamentos candidatos (valores muito frequentes, como silêncio, não contam);
   - para cada alinhamento, os pontos que diferem em ≤ 6 bits contam como iguais, e vale a maior sequência com buracos de até 3,5 s;
   - as pontas da sequência são aparadas até onde a maioria do ~1 s seguinte casa, para que pontos soltos não estiquem a sequência;
   - **limites:** abertura de 15 s a 3 min; créditos com pelo menos 15 s.
3. **Quando roda:** em segundo plano depois de cada scan (gancho `AfterScan` do scanner), **um FFmpeg por vez**.
   - Os fingerprints ficam na tabela `fingerprints`, com a chave `tamanho:mtime:versão`. Um episódio novo só gera o dele; a série é recomparada, o que é só CPU.
   - Mudar janela ou parâmetro: basta subir a `version` e tudo é refeito.
4. **No player:**
   - **Abertura:** o botão "Pular abertura" aparece enquanto ela passa, e o OK do controle pula; play/pause continua na tecla própria.
   - **Créditos:** o card "Próximo episódio" aparece no início deles, e a contagem de 15 s leva ao próximo episódio (com o autoplay ligado).

## Consequências

- **Medido na biblioteca real (HD, 34 episódios): ~2 min**, uns 4 s por episódio.
  - **Origem:** abertura de ~2 min achada em todos os 20 episódios.
  - **Obi-Wan:** ~37 s em todos os 6.
  - **Episódio sem par na série:** nenhum marcador, como esperado.
- **Créditos mais longos que a janela de 4 min** (comum na Disney+) são marcados no começo da janela, e não no começo real. O card aparece mais tarde, mas ainda durante os créditos.
- **Usa sempre a primeira faixa de áudio.** Se episódios da mesma série alternarem a ordem das faixas, a comparação pode falhar.
- **Fora do escopo (Backlog):** pular a abertura automaticamente, corrigir a marcação à mão e pular o recap.
