# ADR 006 — Transcodificação HLS sob demanda

- **Status:** aceito
- **Data:** 31/10/2026

## Contexto

A Sprint 5 faz qualquer arquivo tocar em qualquer cliente. Direct play continua sendo a regra (AGENTS.md §5.1). Quando o cliente não suporta o contêiner, o codec ou a faixa de áudio escolhida, o servidor entrega HLS fMP4 gerado pelo FFmpeg na hora. Tudo isso precisa funcionar com seek, com várias faixas de áudio e sem deixar processos órfãos.

## Decisões

1. **A decisão é do servidor**, a partir dos tokens que o cliente declara em `POST /api/media/{id}/playback`. O frontend descobre esses tokens com `canPlayType` (contêineres) e `MediaSource.isTypeSupported` (codecs em fMP4); o user-agent nunca entra (`internal/stream/plan.go`).
   - **Direct play:** contêiner, vídeo e áudio suportados, e a faixa pedida é a padrão do arquivo.
   - **Senão, HLS:** o vídeo é copiado se for h264/hevc/av1 suportado pelo cliente (HEVC com `hvc1`, por causa do Safari); se não, vira H.264 8-bit com no máximo 1080p. O áudio é copiado se for aac/ac3/eac3/mp3 suportado; se não, vira AAC estéreo.
2. **Uma sessão por ponto de início, com a playlist `EVENT` escrita pelo próprio FFmpeg.** O player soma o `start` da sessão ao tempo do `<video>`. O seek dentro do que já está pronto é nativo; além disso, o player pede uma sessão nova naquele ponto. Trocar a faixa de áudio também abre uma sessão nova.
   - **Rejeitado: playlist VOD inteira de saída com segmentos fixos** (o modelo do Jellyfin). No remux, o FFmpeg só corta nos keyframes do arquivo. Com GOP longo, os segmentos "fixos" escorregam e o número do segmento deixa de bater com o tempo. Corrigir isso exigiria ler o índice do MKV (Cues) e do MP4 (stss).
   - **No remux, a sessão começa num keyframe:** um `ffprobe -read_intervals` (~65 ms) acha o keyframe, o FFmpeg começa nele e o `start` devolvido é exato. Na conversão, o seek do FFmpeg já é preciso.
   - **O seek no lugar vai só até onde o hls.js conhece da playlist.** Além disso o navegador corta o seek, então abre-se uma sessão nova (~0,2 s num remux).
3. **Ciclo de vida:**
   - **Chave:** sha256 de (id, caminho, tamanho, mtime, plano, faixa, início); o diretório é `DATA_DIR/transcode/<chave>`.
   - **Reuso:** sessão rodando ou completa (`#EXT-X-ENDLIST`) é reaproveitada, e é isso que faz o cache.
   - **Encerramento:** 20 s sem pedido de playlist ou segmento encerram o FFmpeg; enquanto o FFmpeg trabalha, o hls.js relê a playlist mesmo pausado. O player também manda `DELETE` (keepalive) ao sair ou trocar de sessão. Saída incompleta é apagada.
   - **Limites:** `MAX_TRANSCODES` limita só as sessões que **codificam vídeo**; remux é barato. `TRANSCODE_CACHE_MB` apaga as sessões completas menos usadas primeiro.
4. **GPU detectada na inicialização, em segundo plano:** uma codificação de teste de 0,2 s, na ordem nvenc → qsv → amf (Windows), nvenc → vaapi → qsv (Linux) e videotoolbox (macOS); depois libx264. A decodificação usa `-hwaccel auto`, com software como reserva automática.
5. **O FFmpeg reescreve a playlist no lugar,** sem `temp_file`: no Windows o rename falha se o servidor estiver lendo o arquivo. O servidor relê quando a leitura vem truncada. Os caminhos de saída vão com `/`, senão o muxer HLS grava o `init.mp4` no diretório de trabalho.
6. **hls.js 1.7 (build `light`)** com import dinâmico só no player: 117 KB gzip fora do bundle inicial. Ele roda no Chromium 76: as APIs novas do dist estão protegidas por `try`.

## Consequências

- **Medido nesta máquina (Radeon RX 6750 XT, AMF):**
  - remux + áudio para AAC: 1º segmento em ~0,15 s, a ~70x o tempo real;
  - conversão de 1080p: 1º segmento em ~1 s, a ~10x;
  - no navegador, do clique até tocar: 0,2 s (troca de áudio) e 1,1–1,4 s (conversão).
- **Fechar a aba encerra o FFmpeg** em ~0,4 s (`DELETE`); com o navegador morto, em ~21 s (ociosidade).
- **O tempo exibido depois de um seek com remux** cai no keyframe anterior, até um GOP antes do pedido.
- **Fora do escopo (Backlog):** *tone mapping* HDR, saída 5.1, *throttling* do FFmpeg e FFmpeg órfão se o servidor for morto à força.
