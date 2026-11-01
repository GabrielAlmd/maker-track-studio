param(
  [Parameter(Mandatory = $true)]
  [string]$InstallerPath
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'assert-windows-diagnostics.ps1')

if (-not (Test-Path -LiteralPath $InstallerPath)) {
  throw "installer not found: $InstallerPath"
}

try {
  $preflight = Invoke-WebRequest -Uri 'http://127.0.0.1:8080/app-config' -UseBasicParsing -TimeoutSec 1
  if ($preflight.StatusCode -eq 200) {
    throw 'localhost:8080 is already serving /app-config before smoke test'
  }
} catch {
  if ($_.Exception.Message -eq 'localhost:8080 is already serving /app-config before smoke test') {
    throw
  }
}

$workDir = Join-Path $env:TEMP ("3dpt-installer-smoke-" + [guid]::NewGuid().ToString())
$installDir = Join-Path $workDir "install-root"
$runtimeDir = Join-Path $workDir "runtime"
$responseFile = Join-Path $workDir "app-config.json"
$stdoutLogFile = Join-Path $workDir "app.stdout.log"
$stderrLogFile = Join-Path $workDir "app.stderr.log"
$process = $null
$previousLocalAppData = $env:LOCALAPPDATA
$previousSQLitePath = $env:SQLITE_PATH
$previousDiagnosticLevel = $env:DIAGNOSTICS_LEVEL
$localDataDir = Join-Path $runtimeDir 'local-app-data'

try {
  New-Item -ItemType Directory -Path $installDir -Force | Out-Null
  New-Item -ItemType Directory -Path $runtimeDir -Force | Out-Null

  $installerArgs = @('/S', "/D=$installDir")
  $installProc = Start-Process -FilePath $InstallerPath -ArgumentList $installerArgs -PassThru -Wait
  if ($installProc.ExitCode -ne 0) {
    throw "installer failed with exit code $($installProc.ExitCode)"
  }

  $binary = @(
    Get-ChildItem -Path $installDir -Recurse -Filter 'Maker Track Studio.exe'
  ) | Select-Object -First 1
  if (-not $binary) {
    Get-ChildItem -Path $installDir -Recurse -Force | Select-Object FullName, Length, LastWriteTime | Format-Table -AutoSize | Out-String -Width 240 | Write-Output
    throw 'installed desktop binary not found after installer execution'
  }

  $versionFile = Join-Path $workDir 'version.json'
  $versionProcess = Start-Process -FilePath $binary.FullName -ArgumentList '--version' -PassThru -Wait -RedirectStandardOutput $versionFile
  if ($versionProcess.ExitCode -ne 0) { throw 'version command failed' }
  $metadata = Get-Content -LiteralPath $versionFile -Raw | ConvertFrom-Json
  if ($metadata.product -ne 'Maker Track Studio' -or
      ($env:BUILD_VERSION -and $metadata.version -ne $env:BUILD_VERSION) -or
      ($env:BUILD_COMMIT -and $metadata.commit -ne $env:BUILD_COMMIT)) {
    throw 'packaged build metadata mismatch'
  }
  $fileInfo = $binary.VersionInfo
  if ($fileInfo.ProductName -ne 'Maker Track Studio' -or
      ($env:WINDOWS_NUMERIC_VERSION -and $fileInfo.FileVersion -ne $env:WINDOWS_NUMERIC_VERSION) -or
      ($env:BUILD_VERSION -and $fileInfo.ProductVersion -ne $env:BUILD_VERSION)) {
    throw "Windows product metadata mismatch: ProductName='$($fileInfo.ProductName)', ProductVersion='$($fileInfo.ProductVersion)', FileVersion='$($fileInfo.FileVersion)'; expected semantic version='$env:BUILD_VERSION', numeric version='$env:WINDOWS_NUMERIC_VERSION'"
  }

  $env:SQLITE_PATH = Join-Path $runtimeDir 'app.db'
  $env:LOCALAPPDATA = $localDataDir
  $env:DIAGNOSTICS_LEVEL = 'INFO'
  $process = Start-Process -FilePath $binary.FullName -PassThru -WindowStyle Hidden -RedirectStandardOutput $stdoutLogFile -RedirectStandardError $stderrLogFile

  $ok = $false
  for ($i = 0; $i -lt 150; $i++) {
    try {
      $response = Invoke-WebRequest -Uri 'http://127.0.0.1:8080/app-config' -UseBasicParsing -TimeoutSec 2
      $response.Content | Set-Content -Path $responseFile -Encoding UTF8
      if ($response.StatusCode -eq 200) {
        $ok = $true
        break
      }
    } catch {
      Start-Sleep -Milliseconds 200
    }
  }

  if (-not $ok) {
    if (Test-Path -LiteralPath $stdoutLogFile) {
      Get-Content -Path $stdoutLogFile
    }
    if (Test-Path -LiteralPath $stderrLogFile) {
      Get-Content -Path $stderrLogFile
    }
    throw 'installed desktop API did not respond'
  }

  $payload = Get-Content -Path $responseFile -Raw | ConvertFrom-Json
  Assert-DesktopDiagnosticLog -LocalDataDirectory $localDataDir -StderrPath $stderrLogFile
  Write-Output ($payload | ConvertTo-Json -Compress)
}
finally {
  $env:LOCALAPPDATA = $previousLocalAppData
  $env:SQLITE_PATH = $previousSQLitePath
  $env:DIAGNOSTICS_LEVEL = $previousDiagnosticLevel
  if ($process -and -not $process.HasExited) {
    Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
  }

  if (Test-Path -LiteralPath $workDir) {
    Remove-Item -LiteralPath $workDir -Recurse -Force -ErrorAction SilentlyContinue
  }
}
