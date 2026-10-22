# Meu Kanban Pro V3

Aplicativo **nativo e offline para Windows 11 (64 bits)** que reúne gerenciamento de projetos, quadro Kanban, Pomodoro com ruído branco e um minijogo Pixel Runner. Desenvolvido em **Go com a API Win32/GDI**, sem Electron, navegador, PowerShell, .NET ou serviço na nuvem.

> **Estado do projeto:** o código-fonte da V3 foi recuperado do pacote original do projeto. A compilação cruzada e os 13 testes unitários disponíveis passaram; a experiência completa **ainda precisa de validação manual em um Windows 11 real** antes de uma divulgação ampla.
> <img width="1220" height="784" alt="kanban_app" src="https://github.com/user-attachments/assets/3ecacfd4-f1f5-4cec-9bc0-89cb5972a362" />


## Recursos

- Projetos e tarefas com título, detalhes, prioridade, prazo e etapas **A fazer / Em andamento / Concluído**.
- Painel de início, edição e pesquisa de tarefas e filtros por projeto.
- Armazenamento automático de tarefas e preferências no próprio computador, além de exportação de backup JSON.
- Pomodoro configurável e ruído branco gerado localmente via áudio do Windows.
- Pixel Runner com seleção de três mascotes pixelados, placar e ritmo progressivo; a partida começa somente quando o usuário seleciona jogar.
- Opção de iniciar o programa com o Windows e respeito à configuração de redução de movimento.

## Executar no Windows 11

1. Abra a pasta [`dist/`](dist/) e obtenha **MeuKanban_Pro_V3.exe** (executável original da V3, sem instalador).
2. Coloque o `.exe` em uma pasta permanente (por exemplo, `Documentos\MeuKanban`).
3. Execute-o no **Windows 11, 64 bits**.
4. Para atualizar a partir de outra versão, faça antes um backup de `%LOCALAPPDATA%\MeuKanban`.

**Aviso:** o `.exe` original foi compilado com Go **1.23.2**, que já não recebe suporte de segurança. Para publicar uma *nova versão para usuários finais*, recompile com uma versão do Go ainda suportada (o workflow automatizado usa `stable`), teste o novo `.exe` no Windows 11 e considere assinatura digital. O arquivo original não é assinado.

## Organização do repositório

| Arquivo / pasta | Função |
| --- | --- |
| `main.go` | Janela Win32, desenho do Kanban, projetos, tarefas e persistência. |
| `pomodoro.go` | Temporizador, configurações e áudio ambiente. |
| `runner.go` | Jogo Pixel Runner e desenho GDI. |
| `internal/layout/` | Cálculos de layout e testes. |
| `internal/runnerphysics/` | Física do minijogo e testes. |
| `assets/` | Ícone e recursos do Windows. |
| `rsrc_windows_amd64.syso` | Recurso Windows já compilado, necessário para incorporar o ícone ao `.exe`. |
| `resources.asm.txt` | Referência de montagem dos recursos, com caminhos relativos. |
| `dist/` | Executável V3 original disponibilizado pelo autor. |
| `build.ps1` / `build.sh` | Scripts de compilação para Windows e Linux/macOS. |
| `.github/workflows/ci.yml` | Testes e build Windows em cada push/PR. |
| `docs/` | Orientações de publicação, avaliação e histórico. |

## Compilar a partir do código

Instale **Go 1.26 ou 1.27, em versão corrigida e suportada** e use uma das opções:

**No Windows (PowerShell):**

```powershell
.\build.ps1
```

**No Linux/macOS (compilação cruzada):**

```bash
bash build.sh
```

**Manual:**

```bash
go test ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-H windowsgui -s -w" -o build/MeuKanban_Pro_V3.exe .
```

No PowerShell do Windows, defina `GOOS`, `GOARCH` e `CGO_ENABLED` com `$env:` ou utilize o `build.ps1`. O programa depende somente da biblioteca padrão de Go e usa o arquivo `.syso` incluído. A compilação não equivale a um teste real da interface, áudio e jogo.

## Dados, privacidade e backup

Não há sincronização automática com a internet no código analisado. Dados são guardados localmente em:

```text
%LOCALAPPDATA%\MeuKanban\
├── kanban.json
├── pomodoro.json
├── pixel-runner.json
└── kanban-backup-anterior.json  (quando aplicável)
```

**Não envie essa pasta ao GitHub:** ela pode conter informações pessoais, projetos e anotações. A exportação do backup de tarefas grava um JSON na pasta Documentos do usuário.

## GitHub e licenciamento

Leia o [guia para publicar](docs/PUBLICAR_NO_GITHUB.md) e a [avaliação técnica](docs/AVALIACAO_TECNICA.md). **Não foi incluída licença open-source**, pois essa é uma decisão do titular do código. Antes de permitir reutilização pública, escolha e acrescente um arquivo `LICENSE`. Publicar o código em um repositório público por si só não equivale a autorizar redistribuição ou alteração por terceiros.

## Estado dos testes

- **13 testes unitários:** aprovados no ambiente Go 1.23.2/Linux (layout e física).
- **Build cruzado Win64:** concluído a partir do código-fonte recuperado.
- **`go vet` para Windows:** uma advertência relacionada a `unsafe.Pointer`/`LPARAM` no manipulador de mensagens Win32; necessita revisão específica (consulte o relatório).
- **Teste real no Windows 11, verificação de antivírus/SmartScreen e assinatura:** não realizados aqui.
