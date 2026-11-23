$ErrorActionPreference = "Stop"

$Repo = "PapaDanielVi/cafebazaar_cli"
$BinaryName = "bazaar.exe"
$Version = $env:BAZAAR_VERSION
$InstallDir = $env:BAZAAR_INSTALL_DIR

if ([string]::IsNullOrWhiteSpace($InstallDir)) {
    $InstallDir = Join-Path $env:LOCALAPPDATA "Programs\bazaar\bin"
}

function Get-Architecture {
    $arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString().ToLowerInvariant()
    switch ($arch) {
        "x64"   { return "amd64" }
        "arm64" { return "arm64" }
        default { throw "Unsupported Windows architecture: $arch" }
    }
}

function Resolve-LatestVersion {
    $headers = @{ "User-Agent" = "bazaar-installer" }
    try {
        $latest = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers $headers
        if ($latest.tag_name) {
            return $latest.tag_name
        }
    } catch {
        # Fall back to prereleases if no stable release exists yet.
    }

    $releases = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases?per_page=1" -Headers $headers
    if ($releases -and $releases.Count -gt 0 -and $releases[0].tag_name) {
        return $releases[0].tag_name
    }
    throw "Could not determine latest release for $Repo."
}

$Arch = Get-Architecture
if ([string]::IsNullOrWhiteSpace($Version)) {
    $Version = Resolve-LatestVersion
}

$VersionNoV = $Version.TrimStart("v")
$ArchiveName = "bazaar_${VersionNoV}_windows_${Arch}.zip"
$BaseUrl = "https://github.com/$Repo/releases/download/$Version"
$ArchiveUrl = "$BaseUrl/$ArchiveName"
$ChecksumsUrl = "$BaseUrl/checksums.txt"

$TempDir = Join-Path ([System.IO.Path]::GetTempPath()) ("bazaar-install-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $TempDir -Force | Out-Null

try {
    $ArchivePath = Join-Path $TempDir $ArchiveName
    $ChecksumsPath = Join-Path $TempDir "checksums.txt"
    $ExtractDir = Join-Path $TempDir "extract"

    Write-Host "Installing bazaar $Version (windows/$Arch)..."
    Invoke-WebRequest -Uri $ArchiveUrl -OutFile $ArchivePath -UseBasicParsing

    try {
        Invoke-WebRequest -Uri $ChecksumsUrl -OutFile $ChecksumsPath -UseBasicParsing
        $expectedLine = Get-Content $ChecksumsPath | Where-Object { $_ -match "\s$([regex]::Escape($ArchiveName))$" } | Select-Object -First 1
        if ($expectedLine) {
            $expectedHash = ($expectedLine -split "\s+")[0].ToLowerInvariant()
            $actualHash = (Get-FileHash -Path $ArchivePath -Algorithm SHA256).Hash.ToLowerInvariant()
            if ($actualHash -ne $expectedHash) {
                throw "Checksum mismatch for $ArchiveName (expected $expectedHash, got $actualHash)"
            }
        }
    } catch {
        if ($_.Exception.Message -like "Checksum mismatch*") {
            throw
        }
        Write-Warning "Could not verify checksums.txt; continuing installation."
    }

    Expand-Archive -Path $ArchivePath -DestinationPath $ExtractDir -Force
    $SourceBin = Join-Path $ExtractDir $BinaryName
    if (-not (Test-Path $SourceBin)) {
        throw "Expected $BinaryName inside archive."
    }

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $TargetBin = Join-Path $InstallDir $BinaryName
    Move-Item -Path $SourceBin -Destination $TargetBin -Force

    $UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $PathEntries = @()
    if ($UserPath) {
        $PathEntries = $UserPath -split ";" | Where-Object { $_ -ne "" }
    }
    if ($PathEntries -notcontains $InstallDir) {
        $NewPath = ($PathEntries + $InstallDir) -join ";"
        [Environment]::SetEnvironmentVariable("Path", $NewPath, "User")
        $env:Path = "$InstallDir;$env:Path"
        Write-Host "Added $InstallDir to your user PATH."
    }

    Write-Host "Installed bazaar to $TargetBin"
    & $TargetBin version
} finally {
    if (Test-Path $TempDir) {
        Remove-Item -Path $TempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}
