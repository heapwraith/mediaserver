# Passo a passo: Samsung The Frame

Roteiro completo para instalar o mediaserver na **The Frame**. Cenário:

- o código fica no **WSL**;
- o servidor e o Tizen Studio rodam no **Windows**;
- a TV está na mesma rede do PC.

Leva uns 40 minutos na primeira vez. Nas próximas, só o passo 6.

Guia geral da Samsung: [samsung.md](samsung.md).

---

## 0. Confira o ano da TV

Na TV: **Configurações → Suporte → Sobre esta TV**. Anote o **código do modelo** (ex.: `QN55LS03BAGXZD`).

A letra logo depois de `LS03` indica o ano:

| Letra | Ano | Tizen | Funciona? |
|---|---|---|---|
| `T` | 2020 | 5.5 | **Não** (abaixo do mínimo, Chromium 76) |
| `A` | 2021 | 6.0 | Sim |
| `B` | 2022 | 6.5 | Sim |
| `C` | 2023 | 7.0 | Sim |
| `D` | 2024 | 8.0 | Sim |
| `F` | 2025 | 9.0 | Sim |

Anote também o **IP da TV**: **Configurações → Geral → Rede → Status da rede → Configurações de IP**.

---

## 1. Gerar o servidor e o app (no WSL)

```bash
cd ~/projetos/netcasa
make build            # inclui dist/mediaserver-windows-amd64.exe
make tv-tizen-files   # pasta do app em dist/tv/tizen

mkdir -p /mnt/c/mediaserver
cp dist/mediaserver-windows-amd64.exe /mnt/c/mediaserver/mediaserver.exe
rm -rf /mnt/c/mediaserver/tv-tizen && cp -r dist/tv/tizen /mnt/c/mediaserver/tv-tizen
```

---

## 2. Servidor no Windows

O servidor roda no Windows, e não no WSL: o WSL fica numa rede interna (172.x) que a TV não enxerga.

No **PowerShell**:

```powershell
winget install Gyan.FFmpeg          # feche e abra o PowerShell depois
ffmpeg -version                     # tem que responder
cd C:\mediaserver
.\mediaserver.exe -media-dir "D:\Midias"   # troque pela sua pasta de vídeos
```

Na primeira vez, o Windows pergunta sobre o Firewall: marque **Redes privadas** e permita. Se a janela não apareceu, rode no **PowerShell como administrador**:

```powershell
New-NetFirewallRule -DisplayName mediaserver -Direction Inbound -Protocol TCP -LocalPort 8080 -Action Allow -Profile Private
```

Descubra o IP do PC com `ipconfig` (o **Endereço IPv4** do Wi-Fi ou da Ethernet, ex.: `192.168.0.10`).

**Teste:** abra `http://IP-DO-PC:8080` no celular, no mesmo Wi-Fi. Se não abrir, a TV também não vai abrir.

> Depois que tudo funcionar, deixe o servidor como serviço (inicia com o Windows). No PowerShell como administrador:
> `cd C:\mediaserver; .\mediaserver.exe service install -media-dir "D:\Midias"`

---

## 3. Modo desenvolvedor na The Frame

1. Saia do **Modo Arte** (aperte o botão Power rapidamente até aparecer a TV).
2. Abra **Apps** (aperte Home e vá até Apps).
3. Na tela de Apps, digite **1 2 3 4 5**.
   - O controle da The Frame não tem números: aperte o botão **123** / das cores e use o teclado numérico que aparece na tela.
4. Ligue **Developer mode** (*On*). Em **Host PC IP**, coloque o **IP do PC** (passo 2) e confirme.
5. **Reinicie a TV:** segure o botão Power do controle até ela desligar e ligar de novo (ou tire da tomada por 30 s).
6. Em Apps, deve aparecer **DEVELOP MODE** no topo.

---

## 4. Tizen Studio (no Windows, uma vez)

1. Baixe e instale o **Tizen Studio**: [developer.samsung.com → Installing TV SDK](https://developer.samsung.com/smarttv/develop/getting-started/setting-up-sdk/installing-tv-sdk.html). Use a pasta padrão `C:\tizen-studio`.
2. Abra o **Package Manager** → aba **Extension SDK** → instale **Samsung Certificate Extension** e **TV Extensions** (a mais nova).
3. Coloque as ferramentas no PATH. No PowerShell (vale para os próximos terminais):
   ```powershell
   setx PATH "$env:PATH;C:\tizen-studio\tools;C:\tizen-studio\tools\ide\bin"
   ```
   Feche e abra o PowerShell.

---

## 5. Conectar a TV e criar o certificado (uma vez)

**Conectar** (PowerShell):

```powershell
sdb connect IP-DA-TV
sdb devices
```

O `sdb devices` deve listar a TV, com um nome tipo `QN55LS03BAGXZD`. Anote esse nome.

**Certificado Samsung:** a TV só instala apps assinados com ele, e o certificado inclui o identificador da TV.

1. Abra o **Tizen Studio → Tools → Certificate Manager**.
2. Clique em **+** → **Samsung** → **TV** → nome do perfil: **`mediaserver`**.
3. **Author certificate:** *Create a new author certificate* → nome e senha (guarde a senha) → entre com a sua **conta Samsung**.
4. **Distributor certificate:** *Create a new distributor certificate* → privilégio **Public**. O **DUID** da TV conectada no passo anterior é preenchido sozinho; se não for, volte e rode `sdb connect`.
5. Conclua. O perfil `mediaserver` fica ativo.

---

## 6. Empacotar e instalar (repita a cada atualização do app)

No PowerShell:

```powershell
cd C:\mediaserver\tv-tizen
tizen package -t wgt -s mediaserver -- .
sdb connect IP-DA-TV
tizen install -n mediaserver.wgt -t NOME-DA-TV
```

`NOME-DA-TV` é o nome que o `sdb devices` mostrou.

Para abrir na hora: `tizen run -p MdSrvTV001.mediaserver -t NOME-DA-TV`. Ou, na TV, abra **Apps → mediaserver** (no fim da lista; dá para mover para a barra inicial).

> Atualizar o servidor não exige reinstalar o app da TV: a interface vem do servidor. Só repita este passo quando a tela de conexão mudar (`make tv-tizen-files`).

---

## 7. Primeiro uso na TV

1. O app abre na tela **Conectar ao servidor** e procura na rede (alguns segundos).
2. Selecione o servidor com as setas e aperte **OK**.
   - Se ele não aparecer, vá até **IP ou nome**, aperte **OK** (abre o teclado da TV), digite o IP do PC e escolha **Conectar**.
3. Escolha ou crie um perfil.
4. Nas próximas vezes, o app abre direto no servidor salvo. Para trocar: **Configurações → Trocar servidor**.

**Controle:**

| Tecla | Ação |
|---|---|
| Setas / OK | Navegar / escolher |
| Voltar | Volta um nível; no Início, pergunta se quer sair |
| Play/Pause | Pausar/continuar |
| Avançar / Retroceder | ±10 s |

O controle da The Frame não tem teclas coloridas físicas. O botão **123** / das cores mostra teclas virtuais na tela, e os atalhos (Vermelho = legendas, Verde = áudio no player) devem funcionar por elas. De qualquer forma, legenda e áudio também ficam no menu do topo do player (seta para cima).

---

## 8. O que conferir e me contar

- [ ] Modelo e ano da TV (passo 0).
- [ ] O servidor apareceu sozinho na lista ou foi preciso digitar o IP?
- [ ] Navegação completa só com o controle: perfil → Início → série → episódio → assistir → Voltar → sair.
- [ ] **Configurações → Este aparelho:** a lista "Reproduz direto" (copie o texto).
- [ ] Um vídeo **H.264 + AAC** e um **HEVC + AAC** tocam? No terminal do servidor, a linha `playback` deve mostrar `"direct":true`.
- [ ] Feche o app (Voltar no Início → Sair), abra de novo: o perfil continua escolhido?
- [ ] Alguma tela com texto pequeno, cortado ou fora do lugar? (foto ajuda)

---

## Problemas comuns

| Sintoma | O que fazer |
|---|---|
| `sdb connect` não encontra a TV | Confira se o modo desenvolvedor está ligado e o **Host PC IP** é o IP atual do PC (ele muda se o roteador der outro IP). Reinicie a TV depois de mudar. |
| `tizen package` reclama do perfil | O nome do perfil no Certificate Manager tem que ser `mediaserver` (ou troque `-s mediaserver` pelo seu). |
| `tizen install` falha com erro de certificado | O certificado de distribuidor não tem o DUID desta TV. Crie de novo com a TV conectada (passo 5). |
| App instala mas mostra "Nenhum servidor encontrado" | O servidor está rodando no Windows (não no WSL)? O celular abre `http://IP-DO-PC:8080`? A porta no app é 8080? |
| "O servidor não abriu o aplicativo" | O servidor respondeu, mas a interface não carregou em 15 s. Rode `make build` de novo e substitua o `.exe` (passo 1). |
| O app sumiu da TV | Apps instalados pelo modo desenvolvedor continuam instalados; confira em Apps, no fim da lista. Se a TV foi restaurada para o padrão de fábrica, refaça o passo 6. |
| Ver o que acontece dentro do app | `sdb shell 0 debug MdSrvTV001.mediaserver` mostra uma porta. No Chrome do PC, abra `chrome://inspect` → *Configure* → `IP-DA-TV:porta`. |
