# LG (webOS)

Instalação sem loja, pelo Developer Mode. Use uma TV LG de 2021 em diante (webOS 6+) e um PC na mesma rede.

## 1. Developer Mode na TV

1. Crie uma conta em [webostv.developer.lge.com](https://webostv.developer.lge.com).
2. Na TV, instale o app **Developer Mode** pela LG Content Store e entre com essa conta.
3. Ligue **Dev Mode Status** (a TV reinicia) e depois **Key Server**.

A sessão do Developer Mode dura algumas dezenas de horas. Renove no próprio app (**Extend**); se ela expirar, os apps instalados por esse modo somem. Para instalação permanente em TV com root, há o Homebrew Channel (não coberto aqui).

## 2. CLI do webOS (no PC)

```bash
npm i -g @webos-tools/cli
ares-setup-device        # add → nome "tv", IP da TV, porta 9922, usuário prisoner
ares-novacom --device tv --getkey    # pede a passphrase mostrada no app Developer Mode
```

## 3. Gerar e instalar

```bash
make tv-webos
ares-install --device tv dist/tv/webos/com.mediaserver.tv_1.0.0_all.ipk
ares-launch --device tv com.mediaserver.tv
```

## Problemas

- **Depurar:** `ares-inspect --device tv --app com.mediaserver.tv --open`.
- **IP da TV não encontrado:** o app usa `luna://com.palm.connectionmanager/getStatus` (ACG `network.query`, declarado no `appinfo.json`). Se falhar, ele procura em 192.168.0.x e 192.168.1.x, ou você digita o endereço.
