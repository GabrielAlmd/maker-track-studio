# Your first print and cost estimate

[Português](#português) · [English](#english)

## English

Use this example in a disposable workspace, not over your existing records.
All names and amounts below are fictional. For your own workspace, enter your
actual costs. Install from the official [Releases](https://github.com/GabrielAlmd/maker-track-studio/releases)
and follow the [Desktop guide](../apps/desktop/README-DISTRIBUTION.md) or
[Self-Hosted guide](SELF_HOSTED.md). v1.0.0 is published; v1.0.1 is in preparation.

### 1. Set starting costs

Open **Settings > Costs & Calculations**. These are defaults copied when creating a
printer. Select **Simple**, set electricity to **€0.20 per hour**, wear to
**€0.30 per hour**, labor to **€12 per hour**, and filament waste factor to
**1.1**. Save changes. Simple mode uses an hourly electricity charge; Advanced
mode instead uses power in watts and your tariff per kWh.

### 2. Add a printer

Open **Printers > Add first printer** (or **Add Printer**). Leave the preset
at **Custom / no preset** and enter name **Guide Printer**, manufacturer
**Example**, model **Demo**. Create the printer. The first active printer becomes
the default. Use **Edit costs** to review the values copied from Settings;
changing defaults later does not update an existing printer.

### 3. Record material you bought

Open **Purchases > Record first purchase** (or **Record Purchase**). Choose a
purchase date and **Create new filament**. Enter brand **Guide Brand**, type
**PLA**, color **Blue**, total price **€20**, and amount **1 kg**. Record the
purchase. If PLA is unavailable, add it through the filament type manager first.
A purchase quantity is in kilograms, while print material usage is in grams.
This single purchase establishes an average material price of **€20/kg**.

### 4. Record a print

Open **Print Jobs > Add first print job** (or **Add Print Job**). Select
**Guide Printer**. Leave the optional model empty, choose a print date, and enter:

| Field | Value | Meaning |
| --- | --- | --- |
| Units printed | 2 | Pieces produced by the whole job |
| Print time | 60 minutes | Total machine running time |
| Labor hours | 0.25 | 15 minutes of your work, not machine time |
| Filament | Guide Brand / PLA / Blue | Material from the purchase |
| Weight used | 100 g | Total material for the whole job, not per piece |

Use **Add filament** to enter the material row, then create the job. Enter total
time and weight once; do not multiply these totals again by the two pieces.
When using a model later, review its automatically estimated time and weight.

### 5. Read the result

The job should have this breakdown (rounding only for display):

| Component | Calculation | Cost |
| --- | --- | --- |
| Material | 100 / 1000 × €20 × 1.1 | €2.20 |
| Electricity | 1 hour × €0.20 | €0.20 |
| Wear | 1 hour × €0.30 | €0.30 |
| Labor | 0.25 hours × €12 | €3.00 |
| Whole job | Sum | **€5.70** |
| Per piece | €5.70 / 2 | **€2.85** |

Reload the page and check the job remains. A missing/unavailable cost is not
zero: check the material purchase and printer assignment before using a result.

A selling-price **margin** is a share of the selling price. For illustration,
a 20% margin on €5.70 is €5.70 / (1 − 0.20) = €7.125, displayed as €7.13.
A 20% **markup** is €5.70 × 1.20 = €6.84. These are different calculations;
this illustration is not a promise that 20% is the application's current tier.

### What can change old results?

- Changing Settings affects new-printer defaults and legacy jobs without a
  saved printer configuration; it does not replace assigned jobs' saved inputs.
- Editing a printer's costs affects new assignments. Reassigning a job captures
  the selected printer's current inputs.
- Job duration, labor, material usage and live average filament purchase prices
  can still change a job's calculated total. A printer snapshot freezes cost
  inputs, not every part of the job.
- Completed Sales orders keep their frozen profitability even if these inputs
  change. See [printer history](PRINTER_PROFILES.md) and [Sales](SALES_AND_ORDERS.md).

Finish by exporting a ZIP in **Settings > Data & Backups**, keeping it away from
the device, and following the [backup and support guide](SUPPORT.md).

## Português

Experimente numa instalação descartável, sem substituir os seus registos.
Os nomes e valores são fictícios. Para utilização real, introduza os seus custos.
Descarregue a aplicação das [Releases oficiais](https://github.com/GabrielAlmd/maker-track-studio/releases)
e siga o [guia Desktop](../apps/desktop/README-DISTRIBUTION.md) ou
[Self-Hosted](SELF_HOSTED.md). A v1.0.0 está publicada; a v1.0.1 está em preparação.

1. Em **Definições > Custos e cálculos**, escolha o modo **Simples**: eletricidade
   **0,20 €/hora**, desgaste **0,30 €/hora**, trabalho **12 €/hora** e fator de
   desperdício **1,1**. Guarde. No modo Simples a eletricidade é um valor por hora;
   no Avançado depende da potência em watts e da tarifa por kWh.
2. Em **Impressoras**, adicione **Guide Printer**, fabricante **Example**, modelo
   **Demo**, sem predefinição. A primeira impressora ativa torna-se a padrão.
   Reveja os custos copiados das Definições no editor de custos da impressora.
3. Em **Compras**, registe uma compra com novo filamento: marca **Guide Brand**,
   tipo **PLA**, cor **Blue**, preço total **20 €**, quantidade **1 kg** e uma
   data. Se necessário, crie PLA no gestor de tipos. O preço médio fica **20 €/kg**.
4. Em **Impressões**, crie um trabalho com **Guide Printer**, sem modelo,
   **2 unidades**, **60 minutos**, **0,25 horas de trabalho** e **100 g** do filamento
   comprado. Escolha a data e adicione a linha de filamento. Tempo, peso e trabalho
   são totais da impressão; não os multiplique novamente pelas unidades.
5. Confirme **2,20 €** de material, **0,20 €** de eletricidade, **0,30 €** de desgaste
   e **3,00 €** de trabalho: **5,70 €** no total, ou **2,85 € por peça**.
   Recarregue a página e confirme que o registo permanece. Um custo indisponível
   não significa zero: confirme a compra de material e a impressora atribuída.

Uma margem de 20% corresponde a 5,70 € / (1 − 0,20) = 7,125 €, apresentado como
7,13 €. Um acréscimo de 20% sobre o custo corresponde a 5,70 € × 1,20 = 6,84 €.
O exemplo não implica que 20% seja o escalão configurado na aplicação.

As Definições alteram os valores para novas impressoras e trabalhos antigos sem
configuração guardada. Os trabalhos atribuídos conservam os valores capturados;
alterar os custos da impressora afeta novas atribuições. Mudar a impressora de um
trabalho captura os valores atuais da nova impressora. Alterações de tempo,
trabalho, material e preços médios de compra ainda podem alterar o custo de uma
impressão. As vendas concluídas conservam os seus resultados financeiros.

Exporte um ZIP em **Definições > Dados e cópias de segurança** e guarde uma cópia
fora do dispositivo. Consulte [cópias, recuperação e suporte](SUPPORT.md#português).

## Verification / Verificação

On 2026-11-10, the English workflow was exercised with Chromium against the
shared Go desktop runtime on loopback and a fresh disposable SQLite database,
using the Vite browser UI. Settings, printer, purchase and print were entered
through their forms. Reload retained the print; the real API returned material
€2.20, electricity €0.20, wear €0.30, labor €3.00 and total €5.70. Browser ZIP
export/import preserved that print and removed a synthetic post-export record.

This verifies the local browser/shared-backend workflow, not a packaged Wails
app, native installer or authenticated LAN deployment. Those checks and upgrade
from the published v1.0.0 remain pending in the [readiness record](RELEASE_READINESS_1.0.1.md).

Em 2026-11-10, o percurso em inglês foi verificado em Chromium com a API Go
real e uma base SQLite descartável: total de 5,70 €, persistência após recarregar
e exportação/importação ZIP confirmados. Esta verificação não qualifica os
instaladores, Wails, LAN nem a atualização da v1.0.0; esses pontos continuam
pendentes no registo de preparação.
