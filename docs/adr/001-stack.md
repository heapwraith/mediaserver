# ADR 001 — Escolha da stack

- **Status:** aceito
- **Data:** 31/10/2026

## Contexto

Queremos um servidor de mídia local, rápido e leve (metas no `AGENTS.md` §8), que rode em Windows, Linux e macOS e atenda navegadores de PC e TVs a partir de 2021 (Tizen 6 / webOS 6, Chromium 76). Não há APIs externas de metadados: tudo vem dos arquivos do usuário.

## Decisões

1. **Backend em Go 1.27, binário único sem CGO.** Compilação cruzada trivial para 6 alvos, inicialização em milissegundos, pouca RAM, `http.ServeContent` com Range/`sendfile` na stdlib. `encoding/json/v2` e `uuid` da stdlib evitam dependências. Router `chi` v5: leve, compatível com `net/http`.
2. **SQLite (`modernc.org/sqlite`) em modo WAL.** Banco embutido sem servidor, em Go puro (sem CGO). WAL permite leituras durante o scan; `busy_timeout` evita erros de lock. Migrações `.sql` numeradas e embutidas no binário.
3. **SvelteKit 3 + Svelte 5 com `adapter-static` (SPA).** Bundle pequeno, sem runtime pesado, compila para JS simples. O build é embutido no binário via `embed.FS`; o Go serve o `index.html` como fallback para as rotas do cliente. Um frontend só para PC e TVs.
4. **UnoCSS (`preset-wind3` + `preset-legacy-compat`) + Lightning CSS.** Classes estilo Tailwind geradas no build (zero JS em runtime) e CSS convertido para os alvos do browserslist. Tailwind v4 e `preset-wind4` ficam de fora porque exigem Chrome 111+.
5. **Piso Chromium 76.** `web/.browserslistrc` é a fonte única dos alvos de JS (Vite) e CSS (Lightning CSS). Recursos sem conversão possível (flex `gap`, `aspect-ratio`, `inset`, `:has()`, container queries, `subgrid`, `@layer`, `:where()`) são bloqueados na `blocklist` do UnoCSS, com teste automatizado.
6. **Player Video.js 10 + hls.js, JASSUB para ASS** (a partir da Sprint 3). Sucessor do Vidstack, suporta HLS em todos os navegadores e legendas ASS estilizadas de anime.
7. **Normalização virtual da biblioteca.** O servidor nunca renomeia, move ou apaga arquivos. O scanner interpreta nomes e pastas e grava no banco a estrutura limpa; correções manuais viram *overrides*.
8. **Qualidade:** `golangci-lint` v2 (fixado em `tools.mod`, roda via `go tool`, sem instalação global), Oxlint + Oxfmt + svelte-check (tsgo) + Vitest no frontend. Tudo exposto pelo `Makefile`.

## Consequências

- Uma única linguagem no servidor e um único artefato por plataforma; instalação = copiar o binário.
- `go test -race` precisa de CGO: ele só é ligado nos testes, nunca nos binários de release.
- O FFmpeg/ffprobe continua sendo binário externo (detectado em runtime), não embutido.
- Usar qualquer recurso CSS da lista proibida exige `@supports` e não pode ser essencial.
- Trocar o nome provisório `mediaserver` será uma mudança única e mecânica (módulo Go, binário, textos).
