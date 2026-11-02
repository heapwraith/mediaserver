# Avisos de terceiros

O Telão (`mediaserver`) é distribuído sob a [GPL-3.0-or-later](LICENSE). Ele usa os componentes abaixo, cada um sob a sua licença. Verificado em 02/11/2026 nos `LICENSE` das versões do `go.sum` e do `package-lock.json`. Atualize ao subir versões.

## Go (compilado no binário)

| Módulo | Licença |
|---|---|
| github.com/go-chi/chi/v5 | MIT |
| github.com/fsnotify/fsnotify | BSD-3-Clause |
| github.com/pelletier/go-toml/v2 | MIT |
| golang.org/x/sys, golang.org/x/text | BSD-3-Clause |
| github.com/google/uuid | BSD-3-Clause |
| modernc.org/sqlite, libc, mathutil, memory | BSD-3-Clause |
| github.com/remyoudompheng/bigfft | BSD-3-Clause |
| github.com/dustin/go-humanize | MIT |
| github.com/mattn/go-isatty | MIT |
| github.com/ncruces/go-strftime | MIT |

O SQLite em si é de domínio público.

## Frontend (vai no build embutido)

| Pacote | Licença | Observação |
|---|---|---|
| svelte, @sveltejs/kit (runtime) | MIT | |
| hls.js | Apache-2.0 | |
| lrud | MIT | |
| jassub | LGPL-2.1-or-later (+ FreeType FTL/GPL-2, libass ISC, HarfBuzz, FriBidi LGPL, outras) | Renderizador ASS em WebAssembly. Código-fonte em https://github.com/ThaUnknown/jassub |
| rvfc-polyfill (via jassub) | **GPL-3.0** | Motivo da escolha da GPL-3.0 para o projeto ([ADR 014](docs/adr/014-licenca.md)) |
| @fontsource/bebas-neue | OFL-1.1 | Fonte Bebas Neue |
| @fontsource-variable/archivo | OFL-1.1 | Fonte Archivo |

Só de build (não vão no que se distribui): Vite, UnoCSS, Lightning CSS (MPL-2.0), caniuse-lite (CC-BY-4.0), Oxlint, Oxfmt, Vitest, TypeScript, svelte-check.

## Programas externos (não são ligados ao código; o servidor os chama como processos)

| Programa | Licença | Observação |
|---|---|---|
| FFmpeg / ffprobe 9.x | LGPL/GPL conforme a compilação | A imagem Docker usa `--enable-gpl --enable-version3` (GPL-3.0). Fonte: https://ffmpeg.org/releases/ |
| x264, x265 | GPL-2.0-or-later | Dentro do FFmpeg da imagem |
| libvpx, dav1d, libopus, libwebp | BSD | Dentro do FFmpeg da imagem |
| LAME | LGPL-2.0+ | Dentro do FFmpeg da imagem |
| libass | ISC | Dentro do FFmpeg da imagem |
| Chromaprint 1.6.2 | MIT (com KissFFT) | Dentro do FFmpeg da imagem |
| nv-codec-headers | MIT | Só cabeçalhos; o driver NVIDIA **não** é distribuído |

## Apps de TV

Os pacotes `.wgt`, `.ipk` e `.apk` contêm só a tela de conexão do projeto (`web/src/shell`) e o código das pastas `tv/`. O wrapper Android usa só o SDK do Android (Apache-2.0) e o Gradle Wrapper (Apache-2.0).

## Marcas

Samsung, Tizen, LG, webOS, Android, Google TV, Fire TV, Amazon, TMDB e as demais marcas citadas pertencem aos seus donos e aparecem só para dizer com o que o projeto funciona. Não há afiliação nem endosso.
