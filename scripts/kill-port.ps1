param(
    [int]$Port = 8080
)

$connections = Get-NetTCPConnection -LocalPort $Port -ErrorAction SilentlyContinue

if (-not $connections) {
    Write-Host "No process is listening on port $Port."
    exit 0
}

$connections |
    Select-Object -ExpandProperty OwningProcess -Unique |
    ForEach-Object {
        Stop-Process -Id $_ -Force -ErrorAction SilentlyContinue
        Write-Host "Stopped process $_ on port $Port."
    }
