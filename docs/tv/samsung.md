# Samsung (Tizen)

Instalação sem loja, pelo modo desenvolvedor. Use uma TV Samsung de 2021 em diante (Tizen 6+) e um PC na mesma rede.

## 1. Modo desenvolvedor na TV

1. Abra **Apps** na TV.
2. Digite `1` `2` `3` `4` `5` no controle (no controle sem números, use o teclado da tela).
3. Ligue **Developer mode** e, em **Host PC IP**, coloque o IP do PC que vai instalar o app.
4. Desligue a TV da tomada por alguns segundos e ligue de novo.

## 2. Tizen Studio (no PC)

1. Instale o [Tizen Studio](https://developer.samsung.com/smarttv/develop/getting-started/setting-up-sdk/installing-tv-sdk.html). A versão CLI basta.
2. No **Package Manager**, em *Extension SDK*, instale **Samsung TV Extensions** (inclui o *Samsung Certificate Extension*).
3. Coloque no PATH as pastas `tizen-studio/tools` (`sdb`) e `tizen-studio/tools/ide/bin` (`tizen`).

## 3. Conectar a TV

```bash
sdb connect IP-DA-TV
sdb devices              # anote o nome do aparelho (ex.: QN55Q60AAGXZD)
```

## 4. Certificado Samsung (uma vez)

A TV só instala apps assinados com um certificado Samsung que inclua o identificador dela (DUID).

1. Abra o **Certificate Manager** do Tizen Studio, com a TV conectada (passo 3).
2. Crie um perfil **Samsung**, tipo **TV**, com o nome `mediaserver`.
3. Gere o certificado de autor e o de distribuidor, fazendo login com a sua conta Samsung. O DUID da TV conectada entra no certificado de distribuidor.

Outro nome de perfil: `make tv-tizen TIZEN_PROFILE=meu-perfil`.

## 5. Gerar e instalar

```bash
make tv-tizen                                                   # dist/tv/tizen/mediaserver.wgt
tizen install -n mediaserver.wgt -t NOME-DO-APARELHO -- dist/tv/tizen
tizen run -p MdSrvTV001.mediaserver -t NOME-DO-APARELHO
```

O app aparece em **Apps** como "mediaserver". Ele continua instalado mesmo depois de desligar o modo desenvolvedor.

## Problemas

- **"Nenhum servidor encontrado":** confira se o servidor abre no navegador do celular (veja [README](README.md)) e se a porta do campo bate com a do servidor.
- **Teclas de mídia ou coloridas sem efeito:** elas precisam ser registradas pela página do app (já é feito). Se continuar, confira se `http://tizen.org/privilege/tv.inputdevice` está no `config.xml`.
- **Depurar:** `sdb shell 0 debug MdSrvTV001.mediaserver` mostra uma porta. Abra `chrome://inspect` no PC e adicione `IP-DA-TV:porta`.
- **Perfil some ao reabrir:** é o risco de storage particionado anotado no [ADR 008](../adr/008-tv-apps.md). Avise com o ano/modelo da TV.
