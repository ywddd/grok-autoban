$ErrorActionPreference = "Stop"

$go = Get-Command go -ErrorAction SilentlyContinue
if (-not $go) {
    $go = Get-Command ".\.tools\go\bin\go.exe" -ErrorAction SilentlyContinue
}
if (-not $go) {
    throw "Go was not found. Install Go 1.21 or newer."
}

$env:CGO_ENABLED = "1"
if (-not $env:CC) {
    $gcc = Get-Command gcc -ErrorAction SilentlyContinue
    if ($gcc) { $env:CC = $gcc.Source }
}
if (-not $env:CC) {
    throw "A C compiler was not found. Install MinGW-w64 or LLVM-MinGW."
}

& $go.Source build -buildmode=c-shared -o "grok-autoban.dll" .
if ($LASTEXITCODE -ne 0) {
    throw "DLL build failed."
}
Write-Host "Built $(Resolve-Path .\grok-autoban.dll)"
