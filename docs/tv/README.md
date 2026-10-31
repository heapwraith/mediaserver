# Apps de TV

O app da TV é **leve**: traz só a tela "Conectar ao servidor". Depois de conectar, ele mostra a mesma interface que o navegador, servida pelo mediaserver. Atualizar o servidor atualiza todas as TVs, sem reinstalar ([ADR 008](../adr/008-tv-apps.md)).

| TV | Pacote | Guia |
|---|---|---|
| Samsung (Tizen 6+, 2021 em diante) | `.wgt` | [samsung.md](samsung.md) |
| LG (webOS 6+, 2021 em diante) | `.ipk` | [lg.md](lg.md) |
| Android TV, Google TV, Fire TV (TCL, Sony, Philips…) | `.apk` | [android.md](android.md) |

## Antes de instalar

1. **O servidor precisa ser alcançável pela TV.** Abra `http://IP-DO-PC:8080` no celular, na mesma rede Wi-Fi. Se não abrir, a TV também não vai conseguir.
   - **Windows:** libere a porta 8080 no Firewall (rede privada).
   - **WSL 2:** por padrão, o WSL fica numa rede interna (172.x). Rode o servidor no Windows ou ative o modo espelhado (`networkingMode=mirrored` no `.wslconfig`).
2. Gere os pacotes:

```bash
make tv-tizen      # Samsung → dist/tv/tizen/mediaserver.wgt
make tv-webos      # LG      → dist/tv/webos/com.mediaserver.tv_1.0.0_all.ipk
make tv-android    # Android → dist/tv/mediaserver-tv.apk
```

## Primeira abertura

- O app procura servidores na rede da TV (`/api/health` em cada endereço, na porta do campo "Porta"). Os encontrados aparecem na lista; escolha com OK.
- Se nada aparecer, digite o IP do PC e a porta. Para digitar, aperte OK no campo: o teclado da TV abre.
- O endereço fica salvo. Nas próximas vezes, o app abre direto no servidor. Se ele estiver desligado, a tela de conexão volta com o erro.
- Para trocar de servidor: **Configurações → Trocar servidor**.

## Controle remoto

| Tecla | Ação |
|---|---|
| Setas / OK | Navegar / escolher |
| Voltar | Fecha o nível atual (player → detalhes → início). No Início, pergunta se quer sair |
| Play, Pause, Play/Pause | Controlam o vídeo |
| Avançar / Retroceder | ±10 s |
| Stop | Sai do player |
| Vermelho / Verde (no player) | Legendas / áudio |
| Amarelo / Azul | Buscar / Início |

## Conferir na TV (aceite da Sprint 7)

- [ ] Navegação completa só com o controle: perfil → início → série → episódio → assistir → voltar → sair.
- [ ] H.264 + AAC e HEVC + AAC tocam em **direct play**. Em **Configurações → Este aparelho**, os formatos devem incluir `h264`, `hevc` e `aac`. No log do servidor, a linha `playback` deve ter `direct=true` (com os `caps` que a TV declarou).
- [ ] Fechar e reabrir o app mantém o perfil e as configurações.
- [ ] Layout sem quebras (a TV de 2021 usa Chromium 76).
