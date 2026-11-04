# Installs Rainbow Statusline for the current user (Windows). No administrator
# rights. The install folder is added to your user PATH, and the script says so.
#
#   irm https://raw.githubusercontent.com/milczarekIT/rainbow-statusline/main/install.ps1 | iex
#
# RAINBOW_STATUSLINE_VERSION   tag to install, e.g. v1.0.0 (default: latest)
# RAINBOW_STATUSLINE_DIR       install folder (default: %LOCALAPPDATA%\Programs\rainbow-statusline)
# RAINBOW_STATUSLINE_BASE_URL  download base, or a local folder, holding the archive and checksums.txt
#                              (default: the GitHub release; mostly for testing)
function Install-RainbowStatusline {
    # Set here, not at the top of the file: under "irm | iex" a top-level setting
    # would change the user's own session.
    $previousPreference = $ErrorActionPreference
    $ErrorActionPreference = 'Stop'
    try {
        $repo = 'milczarekIT/rainbow-statusline'
        $name = 'rainbow-statusline'

        $cpu = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
        switch ($cpu) {
            'AMD64' { $arch = 'amd64' }
            'ARM64' { $arch = 'arm64' }
            default { throw "unsupported CPU: $cpu" }
        }

        if ($env:RAINBOW_STATUSLINE_BASE_URL) { $base = $env:RAINBOW_STATUSLINE_BASE_URL }
        elseif ($env:RAINBOW_STATUSLINE_VERSION) { $base = "https://github.com/$repo/releases/download/$($env:RAINBOW_STATUSLINE_VERSION)" }
        else { $base = "https://github.com/$repo/releases/latest/download" }
        if ($env:RAINBOW_STATUSLINE_DIR) { $dir = $env:RAINBOW_STATUSLINE_DIR }
        else { $dir = Join-Path $env:LOCALAPPDATA "Programs\$name" }
        $archive = "${name}_windows_${arch}.zip"
        $target = Join-Path $dir "$name.exe"

        $tmp = Join-Path ([IO.Path]::GetTempPath()) ([IO.Path]::GetRandomFileName())
        New-Item -ItemType Directory -Path $tmp | Out-Null
        try {
            foreach ($file in $archive, 'checksums.txt') {
                $out = Join-Path $tmp $file
                try {
                    if (Test-Path -LiteralPath $base -PathType Container) {
                        Copy-Item -LiteralPath (Join-Path $base $file) -Destination $out
                    } else {
                        Invoke-WebRequest -UseBasicParsing -Uri "$base/$file" -OutFile $out
                    }
                } catch {
                    throw "could not download $base/$file"
                }
            }

            $line = Get-Content (Join-Path $tmp 'checksums.txt') | Where-Object { $_ -match "\s$([regex]::Escape($archive))$" } | Select-Object -First 1
            if (-not $line) { throw "checksums.txt has no entry for $archive" }
            $want = ($line -split '\s+')[0].ToLower()
            $got = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $tmp $archive)).Hash.ToLower()
            if ($want -ne $got) { throw "checksum mismatch for $archive (expected $want, got $got); nothing was installed" }

            Expand-Archive -LiteralPath (Join-Path $tmp $archive) -DestinationPath (Join-Path $tmp 'x')
            New-Item -ItemType Directory -Force -Path $dir | Out-Null
            Copy-Item -LiteralPath (Join-Path $tmp "x\$name.exe") -Destination "$target.new" -Force
            Unblock-File -LiteralPath "$target.new"
            Move-Item -LiteralPath "$target.new" -Destination $target -Force
        } finally {
            Remove-Item -Recurse -Force -LiteralPath $tmp -ErrorAction SilentlyContinue
            Remove-Item -Force -LiteralPath "$target.new" -ErrorAction SilentlyContinue
        }

        Write-Host "Installed $target"
        # The registry, not GetEnvironmentVariable: that expands %VAR% entries, and
        # writing them back would flatten them for good. @() keeps one entry an array.
        $key = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Environment')
        try {
            $userPath = $key.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
            $entries = @($userPath -split ';' | Where-Object { $_ })
            if ($entries -notcontains $dir) {
                $key.SetValue('Path', (($entries + $dir) -join ';'), [Microsoft.Win32.RegistryValueKind]::ExpandString)
                # A direct registry write tells nobody; clearing an unused variable
                # this way makes Windows announce the change to new terminals.
                [Environment]::SetEnvironmentVariable('RAINBOW_STATUSLINE_PATH_CHANGED', $null, 'User')
                $env:Path = "$env:Path;$dir"
                Write-Host "Added $dir to your user PATH. Terminals opened before now need a restart to see it."
            }
        } finally {
            $key.Close()
        }
        Write-Host ""
        Write-Host "Next: run  $name  to set it up."
    } finally {
        $ErrorActionPreference = $previousPreference
    }
}

try {
    Install-RainbowStatusline
} catch {
    Write-Host "rainbow-statusline install: $($_.Exception.Message)" -ForegroundColor Red
    # Run as a file, a failure is the exit code; under "irm | iex" there is no
    # file, and exiting would close the user's shell.
    if ($PSCommandPath) { exit 1 }
}
