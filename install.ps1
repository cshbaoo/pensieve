# pensieve 一键安装 (Windows):
#   irm https://raw.githubusercontent.com/cshbaoo/pensieve/main/install.ps1 | iex
# 覆盖:$env:VERSION="v0.3.0"; $env:PENSIEVE_INSTALL="D:\tools"
$ErrorActionPreference = "Stop"

$Repo = "cshbaoo/pensieve"
$InstallDir = if ($env:PENSIEVE_INSTALL) { $env:PENSIEVE_INSTALL } else { Join-Path $env:LOCALAPPDATA "Programs\pensieve" }

# ---- 平台检测 ----
$archRaw = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
$Arch = switch ($archRaw) {
  "AMD64" { "amd64" }
  "ARM64" { "arm64" }
  default { throw "不支持的架构: $archRaw" }
}

# ---- 版本解析 ----
$Version = $env:VERSION
if (-not $Version) {
  $rel = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/latest"
  $Version = $rel.tag_name
}
if (-not $Version) { throw "无法解析最新版本号" }
Write-Host "安装 pensieve $Version (windows/$Arch) ..."

# ---- 下载 + 校验 ----
$Asset = "pensieve-windows-$Arch.exe"
$Url = "https://github.com/$Repo/releases/download/$Version/$Asset"
$SumsUrl = "https://github.com/$Repo/releases/download/$Version/checksums.txt"
$Tmp = Join-Path ([IO.Path]::GetTempPath()) ("pensieve-install-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force $Tmp | Out-Null
try {
  Invoke-WebRequest $Url -OutFile (Join-Path $Tmp $Asset)
  Invoke-WebRequest $SumsUrl -OutFile (Join-Path $Tmp "checksums.txt")
  $Expected = ((Get-Content (Join-Path $Tmp "checksums.txt")) | Where-Object { $_ -match " $Asset`$" } | ForEach-Object { ($_ -split '\s+')[0] })[0]
  if (-not $Expected) { throw "校验清单里没有 $Asset" }
  $Actual = (Get-FileHash (Join-Path $Tmp $Asset) -Algorithm SHA256).Hash.ToLower()
  if ($Expected -ne $Actual) { throw "SHA256 校验失败! expected=$Expected actual=$Actual" }
  Write-Host "✔ 校验通过"

  New-Item -ItemType Directory -Force $InstallDir | Out-Null
  Copy-Item (Join-Path $Tmp $Asset) (Join-Path $InstallDir "pensieve.exe") -Force
} finally {
  Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
}

Write-Host "✔ 已安装: $(Join-Path $InstallDir 'pensieve.exe')"

# ---- PATH 持久化(用户级) ----
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (($userPath -split ';') -notcontains $InstallDir) {
  [Environment]::SetEnvironmentVariable("Path", "$userPath;$InstallDir", "User")
  Write-Host "✔ 已加入用户 PATH(新开终端生效)"
}

& (Join-Path $InstallDir "pensieve.exe") --help | Out-Null
Write-Host "✔ 可执行正常"
Write-Host "下一步: pensieve init → AI 工具接入见 https://github.com/$Repo#mcp"
