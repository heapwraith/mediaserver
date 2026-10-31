# mediaserver 1.0.0

Primeira versão estável: servidor de streaming local para filmes, séries e animes, com apps para TVs Samsung, LG e Android.

## Destaques

- **Biblioteca virtual:** aponte uma pasta organizada ou não; o servidor entende nomes de release, agrupa episódios e manda o que for ambíguo para **Revisar**. Seus arquivos nunca são alterados.
- **Direct play primeiro:** o que o aparelho toca vai direto; o resto é convertido em HLS sob demanda, com GPU quando houver (NVENC, Quick Sync, AMF, VAAPI, VideoToolbox).
- **Perfis** com PIN opcional, "continuar assistindo", próximo episódio automático e legenda padrão por perfil.
- **Legendas** SRT/VTT e ASS estilizadas (anime), embutidas ou ao lado do vídeo, com as fontes do MKV.
- **TVs:** apps leves para Samsung (Tizen), LG (webOS) e Android TV/Google TV/Fire TV, com descoberta do servidor na rede e controle remoto completo.
- **Uso diário:** roda como serviço (`mediaserver service install`), backup pela interface (`mediaserver restore` para restaurar) e página **Status do servidor**.

## Arquivos

| Arquivo | Para |
|---|---|
| `mediaserver-windows-amd64.exe` / `-arm64.exe` | Windows |
| `mediaserver-darwin-arm64` / `-amd64` | macOS (Apple Silicon / Intel) |
| `mediaserver-linux-amd64` / `-arm64` | Linux (inclui Raspberry Pi 4/5 de 64 bits) |
| `mediaserver.wgt` | TV Samsung ([guia](https://github.com/heapwraith/mediaserver/blob/main/docs/tv/samsung.md)) |
| `com.mediaserver.tv_1.0.0_all.ipk` | TV LG ([guia](https://github.com/heapwraith/mediaserver/blob/main/docs/tv/lg.md)) |
| `mediaserver-tv.apk` | Android TV, Google TV, Fire TV ([guia](https://github.com/heapwraith/mediaserver/blob/main/docs/tv/android.md)) |
| `SHA256SUMS` | Conferir os downloads: `sha256sum -c SHA256SUMS` |

Instalação e primeiro uso: [README](https://github.com/heapwraith/mediaserver#instalação). Requer FFmpeg 9+ para imagens e conversão.
