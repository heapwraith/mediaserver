# ADR 004 — Frontend: player próprio, navegação e compatibilidade com Chromium 76

- **Status:** aceito
- **Data:** 31/10/2026

## Contexto

A Sprint 3 entrega a interface web, que também será a das TVs (piso **Chromium 76**, AGENTS.md §6). O AGENTS.md previa o **Video.js 10** como player. A documentação dele (`browser-support.md`, v10.2.0) declara piso **Chrome 111** (JS ES2022 e `adoptedStyleSheets`), e as skins exigem `@layer`, `:has()`, `@container` e `:where()` e usam `backdrop-filter`, tudo proibido no §6/§7 e sem fallback.

## Decisões

1. **Player próprio sobre o `<video>` nativo** (rota `assistir/[mediaId]`), no lugar do Video.js 10. Ele tem sobreposição que some em 3 s, barra de progresso, ±10 s, play/pause e menu de áudio. O hls.js (Sprint 5) e o JASSUB (Sprint 6) se ligam direto no mesmo `<video>`. A troca de faixa de áudio usa `audioTracks` onde ele existe (Safari); nos outros navegadores ela vem com a transcodificação.
2. **hls.js adiado para a Sprint 5:** direct play não precisa dele, e ele sozinho estouraria o orçamento de 150 KB.
3. **Navegação com lrud reconstruído a partir do DOM.** A árvore é refeita na ordem do documento quando elementos focáveis montam ou desmontam. A ordem de registro do lrud passa a bater com a tela, qualquer que seja a ordem de montagem do Svelte. Fileiras e grades são **um nó só** e movem o índice internamente (`lib/ui/move.ts`), o que mantém a árvore pequena e permite virtualizar os cards.
4. **`:where()` do Svelte 5:** o Svelte escopa alguns seletores com `:where(.svelte-xyz)`. O Chromium 76 descarta a regra inteira por causa disso. Um plugin do Vite troca, no build, por `.svelte-xyz` (uma classe a mais de especificidade).
5. **Checagem automática do build** (`web/scripts/check-build.ts`, roda no `make check`): mede o JS que a página inicial carrega (gzip) contra 150 KB e procura no CSS final recursos fora do Chromium 76 e blur. `gap` só é aceito em regra com `display:grid` (aproximação).
6. **`unicorn/no-array-sort` desligada:** a alternativa sugerida, `toSorted()`, só existe a partir do Chrome 110.

## Consequências

- Bundle inicial medido: **51,8 KB gzip**.
- Os controles do player são nossos: legendas, miniaturas na barra e Chromecast precisam ser feitos à mão quando chegarem.
- O AGENTS.md (§2) foi atualizado para o player próprio.
