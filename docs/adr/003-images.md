# ADR 003 — API de leitura, direct play e pipeline de imagens

- **Status:** aceito
- **Data:** 31/10/2026

## Contexto

A Sprint 2 expõe a biblioteca, toca arquivos no navegador e gera as imagens dos cards (AGENTS.md §7). Metas: listagem < 50 ms p95 com 10 mil itens, nenhum arquivo fora de `MEDIA_DIR` servido, sem `filter: blur()` no cliente.

## Decisões

1. **Leitura em `internal/library`** (`Store`), separada do scanner, que só escreve.
2. **Paginação pelo índice:** a página é escolhida numa subquery só com `rowid` (os índices de `sort_title`/`added_at` já carregam o `rowid`) e só as linhas da página fazem JOIN. Medido com 10 mil filmes: título 1,1 ms, adição 0,5 ms, ano 10 ms (ano usa ordenação temporária por causa dos anos desconhecidos no fim).
3. **`os.Root` para todo acesso a arquivo do usuário** (stdlib): bloqueia `..`, caminho absoluto e symlink que aponta pra fora de `MEDIA_DIR`, mesmo que o caminho venha do banco.
4. **Direct play com `http.ServeContent`** (Range, `sendfile`, `ETag` de mtime + tamanho).
5. **Imagens geradas pelo FFmpeg**, sem dependência nova (a stdlib não codifica WebP; o `x/image` só decodifica). Um único processo gera as 9 variantes (card, backdrop e blur × 480/960/1920) e a cor dominante (frame reduzido a 1×1).
   - Fontes: card = fanart → poster → miniatura local → frame; backdrop/blur = fanart → frame → poster → miniatura.
   - Melhor frame: candidatos a 10/30/50% da duração (filtro `thumbnail`), descartados se escuros ou chapados; vence a maior variância do Laplaciano. Análise em Go com `image/png`.
   - Blur pré-renderizado (`gblur`) no servidor.
   - Cache em `DATA_DIR/images/<id>/<chave>/`, chave = hash de caminho + tamanho + mtime das fontes. Fonte mudou → chave nova. Falha permanente → pasta vazia (não tenta de novo até a fonte mudar).
   - Sob demanda, no máximo 2 jobs simultâneos, pedidos iguais compartilham o job e o FFmpeg é morto quando todos os clientes desistem.
6. **ETag do corpo** nas respostas JSON (304 na revalidação), sem acoplar a API à versão do scanner.

## Consequências

- O primeiro pedido de cada imagem custa ~0,5 s (vídeo 720p); depois é servido do disco. A cor dominante só aparece na listagem depois que o card foi gerado uma vez (pré-geração no Backlog).
- O cache de itens removidos da biblioteca não é limpo ainda (Backlog).
- Card e backdrop iguais (mesma fonte) ficam duplicados no disco (~70 KB por item).
