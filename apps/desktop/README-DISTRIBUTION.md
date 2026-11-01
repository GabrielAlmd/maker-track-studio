# Maker Track Studio — Installation guide / Guia de instalação

[Português](#português) · [English](#english)

## Português

O Maker Track Studio ajuda a registar impressões 3D, materiais e custos. Esta aplicação é distribuída como software open source; consulte as notas da versão para conhecer alterações e limitações conhecidas.

### Antes de começar

- Descarregue apenas ficheiros enviados por uma pessoa de confiança. Se recebeu um checksum, confirme-o antes de instalar.
- Feche a aplicação antes de instalar uma atualização.
- Os seus dados ficam guardados localmente neste computador. Faça uma cópia de segurança antes de trocar de computador ou de testar versões mais recentes.

### Windows 10 ou posterior (64 bits)

1. Abra o ficheiro `maker-track-studio-windows-amd64-<versão>-installer.exe`.
2. Escolha `Português (Portugal)` ou `English` no início. Os restantes passos do instalador usam o idioma escolhido.
3. Confirme o pedido do Windows e siga os passos do instalador.
4. Abra **Maker Track Studio** a partir do Menu Iniciar. Na primeira abertura, a aplicação usa o idioma escolhido no instalador.

Uma instalação silenciosa aceita `/LOCALE=en` ou `/LOCALE=pt-PT`, por exemplo: `maker-track-studio-windows-amd64-<versão>-installer.exe /S /LOCALE=pt-PT`. Sem essa opção, o instalador usa um idioma suportado do Windows quando disponível e, caso contrário, inglês. As atualizações mantêm o idioma já guardado pela aplicação.

Uma versão sem assinatura de código ou sem reputação estabelecida pode mostrar um aviso do Windows. Descarregue instaladores apenas da página oficial de Releases do projeto e confirme o checksum publicado antes de instalar.

O instalador mostra a licença OFL das fontes incluídas. Os arquivos Desktop incluem `THIRD_PARTY_FONTS_OFL-1.1.txt` junto ao programa.

Para remover a aplicação, use **Definições > Aplicações** no Windows. Atualizar ou remover a aplicação não deve apagar os seus dados locais, mas exporte uma cópia de segurança primeiro.

### Linux (64 bits)

1. Extraia o ficheiro `maker-track-studio-linux-amd64-<versão>.tar.gz`.
2. Abra a pasta extraída.
3. Execute o ficheiro `Maker Track Studio`.

Na primeira abertura da versão portátil, escolha `English` ou `Português (Portugal)`. Esta escolha é guardada localmente e não necessita de uma base de dados existente.

Se o gestor de ficheiros não o abrir, pode iniciá-lo num terminal dentro dessa pasta:

```bash
./Maker\ Track\ Studio
```

É necessária uma sessão gráfica Linux com GTK e WebKitGTK instalados. Esta versão não é destinada a WSL nem a servidores sem ambiente gráfico.

### Primeira utilização

Ao abrir a aplicação, configure os valores de custos e comece por criar materiais, impressoras ou trabalhos conforme necessário. Pode alterar o idioma em **Definições > Aspeto > Idioma** sem reinstalar. Todas as funcionalidades ficam disponíveis localmente.

### Cópias de segurança e atualizações

Em **Definições > Dados e cópias de segurança**, escolha **Exportar dados** para guardar um ficheiro ZIP com os dados locais. Guarde esse ZIP num local seguro. Para recuperar dados noutro computador ou depois de um problema, use **Importar cópia** no mesmo ecrã.

Antes de instalar uma atualização:

1. Exporte uma cópia de segurança.
2. Feche a aplicação.
3. Instale a nova versão e confirme que os seus dados continuam visíveis.

Não altere nem copie manualmente os ficheiros internos da base de dados; use sempre a exportação e importação de cópias de segurança.

### Se algo não funcionar

Ao reportar um problema, inclua:

- a versão mostrada na barra lateral da aplicação;
- o seu sistema operativo e versão;
- o que estava a tentar fazer e a mensagem de erro, se existir;
- os passos para reproduzir o problema.

Não envie cópias de segurança, chaves privadas ou dados pessoais num canal público.

## English

Maker Track Studio helps you record 3D prints, materials, and costs. This application is distributed as open source software; read the release notes for changes and known limitations.

### Before you start

- Download files only from someone you trust. If you received a checksum, verify it before installing.
- Close the application before installing an update.
- Your data is stored locally on this computer. Export a backup before changing computers or trying a newer version.

### Windows 10 or later (64-bit)

1. Open `maker-track-studio-windows-amd64-<version>-installer.exe`.
2. Choose `English` or `Português (Portugal)` near the start. The remaining installer steps use the selected language.
3. Confirm the Windows prompt and follow the installer steps.
4. Open **Maker Track Studio** from the Start menu. Its first launch uses the installer language.

Silent installation accepts `/LOCALE=en` or `/LOCALE=pt-PT`, for example: `maker-track-studio-windows-amd64-<version>-installer.exe /S /LOCALE=pt-PT`. Without the option, the installer uses a supported Windows locale when available, otherwise English. Upgrades preserve the application language already saved by the user.

An unsigned release or one without an established reputation may show a Windows warning. Download installers only from the project’s official Releases page and verify the published checksum before installing.

The installer displays the OFL license for the bundled fonts. Desktop archives include `THIRD_PARTY_FONTS_OFL-1.1.txt` next to the application.

To remove the app, use **Settings > Apps** in Windows. Updating or uninstalling should not delete your local data, but export a backup first.

### Linux (64-bit)

1. Extract `maker-track-studio-linux-amd64-<version>.tar.gz`.
2. Open the extracted folder.
3. Run `Maker Track Studio`.

On the first launch of the portable build, choose `English` or `Português (Portugal)`. The preference is stored locally and does not require an existing database.

If your file manager does not open it, start it from a terminal in that folder:

```bash
./Maker\ Track\ Studio
```

A graphical Linux session with GTK and WebKitGTK installed is required. This version is not intended for WSL or servers without a graphical desktop.

### First use

When you open the app, configure cost values and then create materials, printers, or jobs as needed. You can change the language later under **Settings > Appearance > Language** without reinstalling. All features are available locally.

### Backups and updates

In **Settings > Data & Backups**, select **Export Data** to save a ZIP backup of your local data. Keep that ZIP in a safe location. To recover data on another computer or after a problem, use **Import Backup** on the same screen.

Before installing an update:

1. Export a backup.
2. Close the application.
3. Install the new version and confirm that your data is still visible.

Do not edit or copy the internal database files manually; always use backup export and import.

### If something goes wrong

When reporting a problem, include:

- the version shown in the application's sidebar;
- your operating system and version;
- what you were trying to do and any error message;
- the steps needed to reproduce the problem.

Do not send backups, private keys, or personal data through a public channel.
