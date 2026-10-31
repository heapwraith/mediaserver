# Android TV, Google TV e Fire TV

Um `.apk` só para Android TV, Google TV (Chromecast, TCL, Sony, Philips…) e Fire TV. Ele fica instalado de vez, sem modo desenvolvedor com prazo.

## 1. Gerar

Requer o Android SDK (`ANDROID_HOME`) e JDK 17+. Na primeira vez, o Gradle baixa as dependências.

```bash
make tv-android          # dist/tv/mediaserver-tv.apk
```

O APK é assinado com a chave de depuração do PC (`~/.android/debug.keystore`). Atualizar com um APK gerado em **outro** PC exige desinstalar o anterior antes.

## 2. Instalar

### Pelo adb (rede)

1. Android TV/Google TV: **Configurações → Sistema → Sobre**, clique 7 vezes em *Build* e ligue **Depuração USB/rede** nas opções do desenvolvedor.
2. Fire TV: **Configurações → Minha Fire TV → Opções do desenvolvedor → Depuração ADB** e **Apps de fontes desconhecidas**.

```bash
adb connect IP-DA-TV:5555     # aceite a conexão na TV
adb install -r dist/tv/mediaserver-tv.apk
```

### Pelo app Downloader (sem PC)

1. Sirva a pasta no PC: `python3 -m http.server 8000 --directory dist/tv`.
2. Na TV, instale o **Downloader** (AFTVnews) e permita que ele instale apps desconhecidos.
3. Abra `http://IP-DO-PC:8000/mediaserver-tv.apk` e instale.

O app aparece na fileira de apps da TV como "mediaserver", com banner.

## Problemas

- **Depurar:** com o adb conectado, abra `chrome://inspect` no PC. O WebView aparece nos builds de depuração (`./gradlew assembleDebug`).
- **Interface enorme ou minúscula:** o app escala pela largura (`html.tv`); o Android TV informa 960×540 em TVs 1080p, e isso é esperado.
- **TV antiga (Android 5–7) com WebView desatualizado:** atualize o "Android System WebView" pela loja. A interface precisa de Chromium 76+.
