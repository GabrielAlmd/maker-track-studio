# Backups, recovery and support

[Português](#português) · [English](#english)

## English

### Before updating or restoring

1. In **Settings > Data & Backups**, export a ZIP. Keep a separate copy off the
   computer/server, with the app version and export date.
2. Retain the installed version's official artifact and configuration. Close
   Desktop before updating; for Self-Hosted, stop the service and retain cold
   copies of all volumes, including TLS CA storage, as described in the
   [deployment guide](SELF_HOSTED.md#storage-and-lifecycle).
3. Test import in a disposable installation. Compare records, print costs and
   Sales allocations before relying on a backup.

**Import replaces the workspace's database records; it does not merge them.**
Export current data first, stop other users' work, select the intended ZIP and
choose **Import Backup**. After successful restore, refresh other browser devices
and verify records. The runtime retains a recovery ZIP before replacement;
these local snapshots are not a substitute for a separate backup.

To undo a restore, import the pre-restore ZIP or retained recovery ZIP. To roll
back an update, stop the new version and restore the pre-upgrade data with its
matching old version. Later changes will be absent. Never edit a ZIP manifest
or database to force compatibility. See [SQLite recovery](SQLITE_RECOVERY.md).

If the app cannot start, preserve the data directory and any SQLite journal
files. Follow [startup/storage recovery](SQLITE_RECOVERY.md#power-loss-or-disk-failure);
do not delete lock files or overwrite the database while a process owns it.

### Report a problem

Use the [bug report form](https://github.com/GabrielAlmd/maker-track-studio/issues/new?template=bug_report.yml).
Include the Desktop version shown in the sidebar (or the Self-Hosted image
tag/digest), OS and architecture, steps, expected/actual result and any displayed
failure code. If Desktop cannot open, identify the installer/archive version.

Desktop logs are stored at:

- Windows: `%LOCALAPPDATA%\3d-printing-cost-tracker\diagnostics\logs\application.log`.
- Linux: `${XDG_STATE_HOME:-~/.local/state}/3d-printing-cost-tracker/logs/application.log`.

Report a diagnostic failure code such as `DIAG_UNAVAILABLE` if shown. Review any
log excerpt before posting it. Never upload databases, backups, credentials,
private keys or personal data. Report vulnerabilities privately through the
[security policy](../SECURITY.md), not a public issue. For Self-Hosted, include
sanitized service logs and the deployment details requested in its guide.

Local logging is implemented. Automatic diagnostic-report export and next-start
diagnostic recovery remain planned; do not look for an export-report button.
See the [diagnostics guide](DIAGNOSTICS.md) for log rotation and details.

## Português

### Antes de atualizar ou recuperar

1. Exporte um ZIP em **Definições > Dados e cópias de segurança**. Guarde uma
   cópia fora do computador/servidor, com a versão da aplicação e a data.
2. Conserve o instalador/arquivo oficial e a configuração da versão instalada.
   Feche o Desktop antes de atualizar. Em Self-Hosted, pare o serviço e conserve
   cópias dos volumes com o serviço parado, incluindo a autoridade de certificados;
   siga o [guia de instalação](SELF_HOSTED.md#storage-and-lifecycle).
3. Teste a importação numa instalação descartável. Compare registos, custos de
   impressão e alocações de vendas antes de confiar na cópia.

**A importação substitui os registos da base de dados; não junta os dados.**
Exporte primeiro o estado atual, interrompa o trabalho dos restantes utilizadores,
selecione o ZIP e importe. Depois, atualize os outros navegadores e confirme os
registos. A aplicação guarda uma cópia de recuperação antes da substituição,
mas esta não substitui uma cópia independente.

Para desfazer a recuperação, importe a cópia anterior ou o ZIP de recuperação.
Para reverter uma atualização, pare a nova versão e use a versão antiga com os
dados anteriores à atualização; alterações posteriores não estarão presentes.
Não altere o manifesto do ZIP nem a base de dados para forçar compatibilidade.
Se a aplicação não arrancar, conserve a pasta de dados e os ficheiros de diário
SQLite. Siga o [procedimento de recuperação](SQLITE_RECOVERY.md); não elimine
ficheiros de bloqueio nem substitua uma base utilizada por um processo.

### Comunicar um problema

Use o [formulário de erro](https://github.com/GabrielAlmd/maker-track-studio/issues/new?template=bug_report.yml).
Inclua a versão Desktop na barra lateral (ou tag/digest da imagem Self-Hosted),
sistema operativo, arquitetura, passos, resultado esperado/obtido e o código de
falha apresentado. Se o Desktop não abrir, indique a versão do instalador/arquivo.

Os logs Desktop ficam em:

- Windows: `%LOCALAPPDATA%\3d-printing-cost-tracker\diagnostics\logs\application.log`.
- Linux: `${XDG_STATE_HOME:-~/.local/state}/3d-printing-cost-tracker/logs/application.log`.

Se aparecer, comunique o código de diagnóstico, por exemplo `DIAG_UNAVAILABLE`.
Reveja os excertos antes de os publicar. Nunca envie bases de dados, backups,
credenciais, chaves privadas ou dados pessoais. Comunique vulnerabilidades em
privado conforme a [política de segurança](../SECURITY.md). Em Self-Hosted,
inclua logs revistos e os detalhes de instalação pedidos no respetivo guia.

Os logs locais estão implementados. A exportação de relatórios de diagnóstico e
a recuperação de diagnóstico no arranque seguinte continuam planeadas; não existe
um botão para exportar esses relatórios. Consulte [diagnósticos](DIAGNOSTICS.md).
