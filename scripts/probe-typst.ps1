$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ('dd-typst-probe-' + [guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    $url = 'https://github.com/typst/typst/releases/download/v0.15.0/typst-x86_64-pc-windows-msvc.zip'
    $zip = Join-Path $tmp 't.zip'
    Invoke-WebRequest -Uri $url -OutFile $zip
    & "$env:SystemRoot\System32\tar.exe" -xzf $zip -C $tmp
    Get-ChildItem $tmp -Recurse | Select-Object FullName
} finally {
    Remove-Item $tmp -Recurse -Force
}