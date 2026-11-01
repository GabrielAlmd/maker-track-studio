function Assert-DesktopDiagnosticLog {
  param([string]$LocalDataDirectory, [string]$StderrPath)

  $logPath = Join-Path $LocalDataDirectory '3d-printing-cost-tracker/diagnostics/logs/application.log'
  for ($attempt = 0; $attempt -lt 50; $attempt++) {
    if (Test-Path -LiteralPath $logPath) {
      # Match the live writer's sharing requirements; ReadAllText only shares
      # for read and can itself fail while the application has a write handle.
      $stream = [IO.File]::Open($logPath, [IO.FileMode]::Open, [IO.FileAccess]::Read,
        ([IO.FileShare]::ReadWrite -bor [IO.FileShare]::Delete))
      $reader = [IO.StreamReader]::new($stream)
      try { $content = $reader.ReadToEnd() } finally { $reader.Dispose() }
      # The writer may be flushing while we read: inspect complete lines only.
      $lines = $content -split "`n"
      for ($line = 0; $line -lt $lines.Length - 1; $line++) {
        if (-not $lines[$line].Trim()) { continue }
        $entry = $lines[$line] | ConvertFrom-Json
        if ($entry.schema_version -eq 1 -and $entry.event -eq 'app.ready') {
          Write-Output 'Packaged application persisted its ready event.'
          return
        }
      }
    }
    Start-Sleep -Milliseconds 200
  }
  if (Test-Path -LiteralPath $StderrPath) {
    # Print only the safe diagnostic code, never arbitrary stderr content.
    $match = [regex]::Match([IO.File]::ReadAllText($StderrPath), '\bDIAG_[A-Z_]+(?:_[0-9]+)?\b')
    if ($match.Success) { throw "Packaged application logging failed: $($match.Value)" }
  }
  throw 'Packaged application did not persist its ready event in application.log'
}
