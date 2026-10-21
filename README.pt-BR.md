<p align="center">
  <img src="assets/docdrift.png" alt="docdrift" width="320">
</p>

# docdrift

[English](README.md) · **Português**

Ache as mentiras da sua documentação antes que o seu agente de IA acredite nelas.

<p align="center">
  <img src="assets/demo.gif" alt="docdrift acusando um arquivo movido, um link quebrado e um repositório renomeado" width="800">
</p>

README, `CLAUDE.md`, `AGENTS.md` e notas pessoais descrevem uma árvore que não para de mudar. Um
arquivo é renomeado, um repositório troca de dono, uma pasta vai parar dentro de um monorepo, e a
doc continua dizendo a coisa antiga. Uma pessoa desconfia; um agente de código lê a doc como
verdade e age em cima dela.

O `docdrift` extrai do markdown as afirmações que dá para checar e confere cada uma contra o
disco e, se você pedir, contra o GitHub:

- caminhos em código inline: `` `internal/api/server.go` ``, `` `scripts/` ``
- links relativos: `[guia](docs/guia.md)`
- repositórios do GitHub: `github.com/dono/repo`, inclusive dentro de bloco de código
- caminhos da home (`~/.config/x`, `/home/voce/...`), quando você pede

Quando um caminho sumiu, ele procura um arquivo com o mesmo nome em outro lugar do repositório e
sugere para onde foi:

```
AGENTS.md:4: path does not exist: internal/db/postgres.go
    maybe moved to: api/internal/store/postgres.go
AGENTS.md:6: link target does not exist: docs/rollback.md
AGENTS.md:7: GitHub repository was renamed to moby/moby: docker/docker
```

## Instalação

```sh
go install github.com/jvS0uzx/docdrift/cmd/docdrift@latest
```

## Uso

```sh
docdrift                       # todo .md abaixo do diretório atual
docdrift README.md docs/       # arquivos e diretórios específicos
docdrift -github .             # confere também os repositórios (GITHUB_TOKEN ou GH_TOKEN)
docdrift -format json . > drift.json
```

O código de saída é `1` quando há achados, `2` em erro de uso e `0` caso contrário (ou sempre `0`
com `-exit-zero`), então entra no CI sem adaptação.

| Flag | Efeito |
|---|---|
| `-github` | Consulta cada referência `github.com/dono/repo`. Acusa repositório apagado, privado, renomeado ou transferido. |
| `-home` | Checa caminhos `~/` e absolutos dentro da sua home. Feito para notas pessoais sobre a máquina que roda a checagem. |
| `-also DIR` | Diretório extra contra o qual caminhos relativos podem resolver, para docs que descrevem um repositório vizinho. Repetível. |
| `-exclude GLOB` | Pula arquivos e diretórios pelo nome, ex.: `-exclude Releases -exclude 'CHANGELOG*'`. Repetível. |
| `-strict` | Acusa também os caminhos sem âncora (ver abaixo). |
| `-history` | Checa também linhas que falam do passado ou da ausência, e notas inativas. |
| `-format json` | Achados e estatísticas legíveis por máquina. |

## O que ele não acusa, e por quê

Detectar drift é fácil; o difícil é ficar quieto quando a doc está certa. Estes casos são pulados
por padrão e contados na linha de resumo:

- **Nota inativa.** Frontmatter com `status:` desativada, arquivada, histórico, obsoleta ou
  descontinuada (ou `deprecated`, `archived`), ou com essas palavras em `tags:`. A nota inteira é
  registro do passado.
- **Linha histórica.** Uma linha que diz *renomeado*, *removido*, *antigo*, *era* (ou *renamed*,
  *formerly*, *legacy*) descreve o passado de propósito.
- **Linha de negação.** Uma linha que já diz que a coisa não existe: *não existe*, *inexistente*,
  *ausente*, *caminho morto*, *404*, ❌ (ou *does not exist*, *missing*, *gone*). A doc está
  certa.
- **Placeholder.** `<nome>`, `{id}`, `[Título]`, `NNN_nome.sql`, `AAAAMMDD`, `seu-usuario/`,
  `dono/repo`, `00_Sistema/.../x.md`.
- **Caminho gerado.** Tudo o que o `.gitignore` da raiz ignora, mais `node_modules`, `dist`,
  `build`, `target`, `.venv` e afins.
- **Sem âncora.** Se `foo/bar.go` não existe *e* não há `foo/` em lugar nenhum da raiz, a doc
  provavelmente descreve outra árvore, e adivinhar qual seria ruído. `src/sumiu.go` num repositório
  que tem `src/` é acusado. Use `-strict` para ver esses também.
- **Fora de escopo.** Caminho absoluto fora da sua home (`/etc/nginx/...` em geral descreve um
  servidor), caminho da home sem `-home` e referência ao GitHub sem `-github`.
- **Rota de site de documentação.** Link absoluto sem extensão (`/api/authentication`) é página
  de site, não arquivo. Link relativo sem extensão (`../guia/setup`) resolve para `setup.md`,
  `setup.mdx` ou `setup/index.md`, como no Docusaurus e no MkDocs, e `/images/x.png` pode resolver
  a partir de qualquer pasta entre o arquivo e a raiz.
- **Não é repositório.** `docs.github.com/...`, `github.com/user-attachments/...`, badge terminado
  em `.svg` e identificador Go como `github.com/google/uuid.UUID` são lidos pelo que são.

As palavras de passado e de negação são procuradas só na prosa da linha, não nos caminhos: um
arquivo chamado `legacy.go` ou `gone.go` não desliga a checagem.

Os caminhos são tentados em relação ao arquivo markdown, à raiz do repositório e a cada pasta de
primeiro nível da raiz. Assim, uma doc que diz `internal/db.go` <!-- docdrift:ignore -->
num monorepo cujo código fica em `backend/internal/db.go` não vira alarme falso.

### Notas que vivem fora do código

Uma nota num vault do Obsidian ou numa wiki pode dizer qual checkout descreve com a chave `repo:`
no frontmatter. Os caminhos relativos dessa nota resolvem contra ele:

```yaml
---
repo: /home/voce/codigo/meu-servico
---
```

### Silenciando uma linha ou um arquivo

```md
A deploy key fica em `~/.ssh/deploy_key` no servidor. <!-- docdrift:ignore -->
```

`<!-- docdrift:ignore-file -->` em qualquer ponto do arquivo pula o arquivo inteiro. As diretivas
só valem como comentário HTML na prosa: citadas dentro de código, como aqui, não fazem nada.

## CI

Como GitHub Action:

```yaml
- uses: actions/checkout@v4
- uses: jvS0uzx/docdrift@v0.1.2
  with:
    args: -github .
```

Ou instalando o binário:

```yaml
- uses: actions/setup-go@v5
  with:
    go-version: stable
- run: go install github.com/jvS0uzx/docdrift/cmd/docdrift@latest
- run: docdrift -github .
  env:
    GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

## Limitações

- Um caminho relativo só conta se tiver extensão conhecida ou barra no fim. `cmd/app` pode ser
  branch, MIME type ou `dono/repo`, então não é checado.
- Só o `.gitignore` da raiz é lido; `**` no meio de um padrão casa um único segmento.
- As palavras de passado e de negação valem para a linha inteira: uma linha que nega uma coisa e
  cita outra, viva, deixa a segunda sem checagem.
- Nomes de função, portas, variáveis de ambiente e comandos citados na doc ainda não são checados.
