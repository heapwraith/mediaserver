# Política de segurança

O Telão é um servidor **doméstico**, feito para a rede local. Não foi projetado para ficar exposto à internet.

## Como reportar uma vulnerabilidade
Use **Security → Report a vulnerability** neste repositório do GitHub (relato privado). Não abra uma issue pública com detalhes de exploração. Responda com: versão, passos para reproduzir e o impacto que você vê. Em um projeto mantido por voluntários, a resposta pode levar alguns dias.

## O que o projeto protege
- **Nenhuma chamada à internet** por padrão; sem telemetria (só o TMDB, se você ligar).
- **Arquivos:** só dentro de `MEDIA_DIR` (`os.Root`); o servidor nunca altera nem apaga mídia.
- **Rede:** pedidos com `Host` de domínio externo (DNS rebinding) e gravações vindas de outro site (CSRF) são recusados; só páginas `file://` (os apps de TV) podem embutir a interface.
- **Sessões:** todo o acervo (listas, vídeo, imagens, legendas) exige uma sessão de perfil; token assinado (HMAC), com validade, que muda se o PIN mudar; "Sair de todos os aparelhos" troca o segredo.
- **PIN:** PBKDF2-SHA256 com sal; bloqueio crescente por perfil e endereço.
- **Administração:** PIN de administrador opcional para scan, Revisar, TMDB, status, backup e criação de perfis.
- **FFmpeg:** só lê arquivos locais e pipes e só aceita os contêineres que o scanner reconhece; um arquivo de mídia não consegue abrir conexões de rede.
- **Backup:** sem o segredo das sessões, sem a chave do TMDB e sem os PINs.
- **CSP:** a interface só executa os próprios arquivos e o script de inicialização do SvelteKit (autorizado por hash); sem `eval` de texto injetado em página, sem scripts de terceiros, sem iframe externo.
- **PIN esquecido:** `mediaserver reset-pin admin` ou `mediaserver reset-pin <perfil>`, na máquina do servidor.
- **Cadeia de suprimentos:** `govulncheck`, `npm audit`, CodeQL e revisão de dependências no CI; Actions fixadas por hash; Dependabot; binários de release com checksum, SBOM e atestado de procedência (`gh attestation verify`).

## O que NÃO protege (limites conhecidos)
- O tráfego da rede local é **HTTP sem cifra**: quem estiver no caminho (roteador, repetidor, aparelho comprometido) lê o vídeo, os títulos e os tokens. Use cabo/switch próprio ou, para acesso externo, Tailscale.
- Perfil **sem PIN** é aberto a quem estiver na rede, por desenho. Quem definir o PIN de administrador primeiro vira o administrador.
- A TV tem seus próprios serviços de coleta (ACR, publicidade personalizada): desligue-os nas configurações dela.
- A CSP permite `'unsafe-eval'`: o JASSUB (legendas ASS) compila WebAssembly na thread principal, e isso exige `'unsafe-eval'` antes do Chromium 97 (a Tizen 6.5 é Chromium 85). Scripts inline não autorizados e scripts de outros domínios continuam bloqueados.
- HTTPS na rede local não está disponível: as TVs não aceitam uma autoridade certificadora própria, então só funcionaria com um domínio real e certificado público.

Detalhes: [`docs/auditoria-seguranca.md`](docs/auditoria-seguranca.md) e [ADR 015](docs/adr/015-seguranca-rede.md).
