# Como publicar uma versão

O workflow [`release.yml`](../.github/workflows/release.yml) gera tudo, exceto o app da Samsung:

| Arquivo | Origem |
|---|---|
| `mediaserver-{linux,darwin,windows}-{amd64,arm64}[.exe]` | `make build` (sem CGO) |
| `mediaserver-tv.apk` (Android TV, Google TV, Fire TV) | `make tv-android` |
| `com.mediaserver.tv_<versão>_all.ipk` (LG) | `make tv-webos` |
| `SHA256SUMS` | `sha256sum` de todos acima |
| `mediaserver.wgt` (Samsung) | **à mão**: `make tv-tizen` com o seu certificado Samsung ([guia](tv/samsung.md)) |

- **Rodar à mão** (Actions → Release → *Run workflow*): só gera os arquivos e guarda como *artifact* por 7 dias. Serve pra testar; nada é publicado.
- **Tag `v*`**: gera os arquivos e cria um release **rascunho** no GitHub. Ele só fica público quando você clicar em *Publish*.

## Antes da v1.0.0: aceite da Sprint 8

- [ ] **7 dias de uso contínuo** sem reiniciar o servidor (instale como serviço: `mediaserver service install`).
  - Acompanhe em **Configurações → Status do servidor**: a memória não deve crescer sem parar, e "ligado há" mostra o tempo.
- [ ] Metas do [AGENTS.md §8](../AGENTS.md) registradas. As medições feitas no desenvolvimento estão em [sprint-8-verificacao.md](sprint-8-verificacao.md); faltam as que dependem do seu hardware:
  - [ ] Início com conversão por **NVENC** < 2 s até o primeiro segmento (`make load` com FFmpeg + `HWACCEL=nvenc`, ou o log `playback` + o player).
  - [ ] 3 streams simultâneos (2 direct play + 1 conversão) em aparelhos reais.
- [ ] Aceites manuais pendentes das sprints 6 e 7: anime com ASS pesado vs. mpv; Firefox/Safari; Samsung com o controle.

## Chave do app Android (uma vez)

Sem ela, cada release sai assinado com uma chave descartável, e quem atualiza precisa desinstalar o app antes. Gere uma vez e guarde bem; perder a chave obriga todos a reinstalar:

```bash
keytool -genkeypair -v -keystore mediaserver-release.jks -alias mediaserver \
  -keyalg RSA -keysize 4096 -validity 36500
base64 -w0 mediaserver-release.jks    # macOS: base64 -i mediaserver-release.jks
```

No GitHub (Settings → Secrets and variables → Actions), crie:

| Secret | Valor |
|---|---|
| `ANDROID_KEYSTORE_B64` | a saída do `base64` |
| `ANDROID_KEYSTORE_PASSWORD` | senha do keystore |
| `ANDROID_KEY_ALIAS` | `mediaserver` |
| `ANDROID_KEY_PASSWORD` | senha da chave |

## Publicar

1. Confira o aceite acima e atualize as notas em [`release-notes.md`](release-notes.md).
2. Crie e envie a tag:
   ```bash
   git tag -a v1.0.0 -m "mediaserver 1.0.0"
   git push origin v1.0.0
   ```
3. Espere o workflow **Release** terminar (Actions). Ele cria o rascunho `mediaserver v1.0.0`.
4. Gere o app Samsung e anexe ao rascunho:
   ```bash
   make tv-tizen
   gh release upload v1.0.0 dist/tv/tizen/mediaserver.wgt
   ```
5. Revise o rascunho no GitHub e clique em **Publish release**.

A versão vem da tag: `mediaserver -version` mostra `v1.0.0`, e o APK recebe `versionName 1.0.0` / `versionCode 10000`. Os pacotes da LG (`appinfo.json`) e da Samsung (`config.xml`) têm a versão fixa nos arquivos: atualize-os em versões novas.
