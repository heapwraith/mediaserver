# ADR 007 — Legendas WebVTT, ASS e preferência por perfil

- **Status:** aceito; validado no Edge, aceite manual de anime real e Firefox/Safari pendente
- **Data:** 31/10/2026

## Contexto

O scanner já indexa legendas externas e embutidas, idioma, codec e índice da faixa. A coluna `profiles.subtitle_language` já existe. A Sprint 6 liga esses dados ao player, preservando os efeitos ASS de anime e o suporte a TVs com Chromium 76.

## Decisões

1. `SubtitleKind(codec)` classifica as faixas como `text`, `ass` ou `""`. SRT/SubRip, WebVTT e mov_text são convertidos para WebVTT no servidor e exibidos em `<track>`. ASS/SSA usam JASSUB quando o cliente declara `ass`; sem esse token, o servidor força conversão de vídeo com libass e responde `burned: true`. Legendas de imagem não aparecem no menu.
2. O token `ass` exige WebAssembly, OffscreenCanvas, workers de módulo e memória suficiente. O getter de `WorkerOptions.type` detecta suporte sem usar user-agent. Falha ao inicializar o renderizador também permite repetir a sessão sem `ass`.
3. **JASSUB 2.5.23**, já previsto na stack: não há renderizador ASS nativo no navegador. O import é dinâmico; worker, WASM e fonte padrão ficam hospedados no próprio servidor. `queryFonts: false` impede buscas de fontes externas. A pré-otimização do Vite exclui JASSUB para preservar a transformação de URLs dos assets. A versão foi conferida no [package.json oficial](https://github.com/ThaUnknown/jassub/blob/master/package.json); os caminhos de assets e `timeOffset` foram conferidos no [código do renderizador](https://github.com/ThaUnknown/jassub/blob/master/src/jassub.ts).
4. Cada sessão HLS começa em `start = S`, devolvido pelo servidor. WebVTT recebe `?offset=S`: subtrai S dos cues, descarta os encerrados e corta o início em zero. JASSUB recebe `timeOffset = S`, atualizado a cada sessão. No burn-in: `setpts=PTS+S/TB,subtitles=f=…:fontsdir=fonts,setpts=PTS-STARTPTS`.
5. Uma passagem do FFmpeg extrai todas as faixas de texto/ASS e fontes anexadas. Cache: `DATA_DIR/subtitles/<mediaId>/<chave>/`, derivada do arquivo, com marcador `done`. Pedidos concorrentes compartilham um job com timeout de 10 minutos, independente do cancelamento de um único pedido. Nomes de saída são controlados pelo servidor (`s<índice>.ass`, `fonts/<índice>.<ext>`), nunca copiados do nome do anexo. Versões anteriores não são removidas durante uma extração nova, pois podem ter leitores ativos.
6. Texto externo aceita UTF-8, BOM UTF-8, UTF-16 LE/BE com BOM e Windows-1252/Latin-1. O parser ignora blocos NOTE, STYLE e REGION; remove overrides ASS e `<font>`, escapa texto e preserva marcação simples. `offset` não finito, negativo ou fora da capacidade de `time.Duration` é rejeitado.
7. `subtitle_language` aceita `""` ou `^[a-z]{2,3}(-[A-Z]{2})?$`; ausente/null na atualização preserva o valor. Sem preferência, a legenda começa desligada. Com áudio no idioma preferido, só uma faixa forçada desse idioma é escolhida. Nos demais casos, a primeira tradução do idioma, preferindo a não forçada. A comparação usa o subtag primário (`pt` casa com `pt-BR`). Nenhuma migração é necessária.
8. O player aguarda as faixas antes da primeira sessão, filtra legendas de imagem e identifica faixas forçadas. Navegar pelo menu não aplica a escolha até confirmar. Entrar, trocar ou sair do burn-in abre uma sessão nova no tempo atual; legendas renderizadas pelo cliente não exigem conversão de vídeo.

## API

- `GET /api/subtitles/{id}/vtt?offset=S`: `text/vtt; charset=utf-8`.
- `GET /api/subtitles/{id}/ass`: texto ASS/SSA em UTF-8.
- `GET /api/media/{id}/fonts`: lista de URLs locais.
- `GET /api/media/{id}/fonts/{name}`: fonte com nome validado por regex.
- `POST /api/media/{id}/playback`: recebe `subtitle` além de caps, áudio e início; devolve `burned` quando aplicável.
- Perfis: `subtitle_language` em criação, atualização, listagem e sessão.

## Consequências e validação

As medições fornecidas no plano indicaram extração de duas faixas de um MKV de 3 GB em 0,65 s, FFmpeg 9 com libass e acréscimo estimado de 4,5 MB pelos assets JASSUB. A extração não foi medida novamente nesta continuação. O build confirmou 55,6 KB gzip de JavaScript inicial, abaixo do limite de 150 KB; os assets JASSUB foram servidos pelo binário Go com bytes idênticos ao build e MIME correto.

Resultados e passos pendentes estão em [Verificação da Sprint 6](../sprint-6-verificacao.md). A sprint não está encerrada até validar a renderização e o sincronismo no navegador.

Ficam no Backlog: legendas em imagem via overlay, pré-extração após scan, limpeza de caches antigos, fontes de `Fonts/` ao lado de ASS externa, última legenda por mídia e COOP/COEP para multithread. Anime real com ASS pesado no mpv e Firefox/Safari permanecem como validação manual prevista no plano.
