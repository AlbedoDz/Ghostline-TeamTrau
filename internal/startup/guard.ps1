# Ghostline network guard. Task Scheduler runs this as SYSTEM every minute
# while Ghostline is connected. If Ghostline died without cleaning up (an
# antivirus that kills ghostline.exe also quarantines it, and with it the
# watchdog and the --restore task), the DNS still points at 127.0.0.1 and
# nothing answers there. This puts back the DNS and system proxy recorded in
# state.json using only what ships with Windows; if state.json is gone too,
# loopback DNS goes back to DHCP. It never writes state.json:
# Ghostline's own recovery finishes the rest (firewall, certificates) when it
# next starts. Keep this file ASCII: Windows PowerShell reads it as ANSI.
param(
    [Parameter(Mandatory = $true)][string]$State,
    [string]$UserSid = '',
    [string]$TaskName = 'Ghostline Network Guard',
    [string]$Log = ''
)

$ErrorActionPreference = 'Stop'
$loopback = @('127.0.0.1', '::1')

function Write-GuardLog([string]$msg) {
    if (-not $Log) { return }
    try {
        if ((Test-Path -LiteralPath $Log) -and (Get-Item -LiteralPath $Log).Length -gt 512KB) {
            Move-Item -LiteralPath $Log -Destination "$Log.1" -Force
        }
        Add-Content -LiteralPath $Log -Value ('{0} {1}' -f (Get-Date).ToString('o'), $msg) -Encoding UTF8
    } catch { }
}

# Nothing is left to guard: the task removes itself.
function Remove-Guard([string]$why) {
    Write-GuardLog "done ($why); removing task"
    $ErrorActionPreference = 'Continue' # stderr from a native command must not throw
    & schtasks.exe /Delete /TN $TaskName /F *> $null
}

# The recorded process is still Ghostline: same PID and start time.
function Test-GhostlineAlive($procId, $start) {
    if (-not $procId) { return $false }
    $p = Get-Process -Id ([int]$procId) -ErrorAction SilentlyContinue
    if (-not $p) { return $false }
    try {
        if ($start -is [datetime]) { $want = $start }
        else {
            $want = [datetime]::Parse([string]$start, [Globalization.CultureInfo]::InvariantCulture,
                [Globalization.DateTimeStyles]::RoundtripKind)
        }
        if ($want.Year -le 1) { return $true } # no start time recorded
        return [math]::Abs(($p.StartTime.ToUniversalTime() - $want.ToUniversalTime()).TotalSeconds) -lt 2
    } catch {
        return $true # cannot tell: leave it to Ghostline
    }
}

function Test-IP([string]$s) {
    $ip = $null
    return [System.Net.IPAddress]::TryParse($s, [ref]$ip)
}

function Invoke-Netsh([string[]]$netshArgs) {
    $ErrorActionPreference = 'Continue' # judge by the exit code, not stderr
    $out = & netsh.exe @netshArgs 2>&1
    if ($LASTEXITCODE -ne 0) { throw "netsh $($netshArgs -join ' '): $out" }
}

# Same commands as sysdns.NetshSetDNS: by interface index, never by name.
function Set-FamilyDNS([int]$ifIndex, [string]$fam, $servers) {
    $base = @('interface', $fam, 'set', 'dnsservers', "name=$ifIndex")
    if (-not $servers -or $servers.Count -eq 0) {
        Invoke-Netsh ($base + 'source=dhcp')
        return
    }
    Invoke-Netsh ($base + @('source=static', "address=$($servers[0])", 'register=primary', 'validate=no'))
    for ($i = 1; $i -lt $servers.Count; $i++) {
        Invoke-Netsh @('interface', $fam, 'add', 'dnsservers', "name=$ifIndex", "address=$($servers[$i])", "index=$($i + 1)", 'validate=no')
    }
}

# Restores every adapter family that still points at loopback. Returns how
# many are still stuck there.
function Restore-DNS($snapshot) {
    $stuck = 0
    $adapters = @(Get-NetAdapter -IncludeHidden -ErrorAction SilentlyContinue)
    foreach ($s in @($snapshot)) {
        if (-not $s -or [string]$s.guid -notmatch '^\{[0-9A-Fa-f-]{36}\}$') { continue }
        $ad = $adapters | Where-Object { $_.InterfaceGuid -eq $s.guid } | Select-Object -First 1
        if (-not $ad) { continue } # adapter gone
        foreach ($f in @(@{ Name = 'ipv4'; Family = 'IPv4'; DNS = $s.ipv4 }, @{ Name = 'ipv6'; Family = 'IPv6'; DNS = $s.ipv6 })) {
            if (-not $f.DNS -or -not $f.DNS.mode) { continue }
            $cur = @((Get-DnsClientServerAddress -InterfaceIndex $ad.ifIndex -AddressFamily $f.Family -ErrorAction SilentlyContinue).ServerAddresses)
            if (-not ($cur | Where-Object { $loopback -contains $_ })) { continue } # not ours any more
            $servers = @()
            if ($f.DNS.mode -eq 'static') { $servers = @(@($f.DNS.servers) | Where-Object { Test-IP $_ }) }
            try {
                Set-FamilyDNS $ad.ifIndex $f.Name $servers
                Write-GuardLog "restored $($f.Name) DNS on '$($ad.Name)' to $(if ($servers.Count) { $servers -join ',' } else { 'DHCP' })"
            } catch {
                Write-GuardLog "restore $($f.Name) DNS on '$($ad.Name)' failed: $_; trying DHCP"
                try { Set-FamilyDNS $ad.ifIndex $f.Name @() } catch {
                    Write-GuardLog "DHCP on '$($ad.Name)' failed: $_"
                    $stuck++
                }
            }
        }
    }
    try { Clear-DnsClientCache } catch { }
    return $stuck
}

# Rewrites the WinINET DefaultConnectionSettings blob: flags, server, bypass
# and PAC URL, keeping the version and the tail (auto-detect data).
function Set-ConnectionBlob([byte[]]$blob, [uint32]$flags, [string]$server, [string]$bypass, [string]$pac) {
    $off = 12
    for ($i = 0; $i -lt 3; $i++) { $off += 4 + [BitConverter]::ToInt32($blob, $off) }
    $ms = New-Object System.IO.MemoryStream
    $w = New-Object System.IO.BinaryWriter($ms)
    $w.Write([BitConverter]::ToUInt32($blob, 0))
    $w.Write([uint32]([BitConverter]::ToUInt32($blob, 4) + 1))
    $w.Write($flags)
    foreach ($str in @($server, $bypass, $pac)) {
        $b = [Text.Encoding]::ASCII.GetBytes([string]$str)
        $w.Write([int]$b.Length)
        $w.Write($b)
    }
    $w.Write($blob, $off, $blob.Length - $off)
    $w.Flush()
    return , $ms.ToArray()
}

# Puts the user's proxy back if it is still Ghostline's. Returns $true when
# it is (still) Ghostline's and could not be restored.
function Restore-Proxy($sp) {
    if (-not $sp -or -not $sp.set -or $sp.takenOver -or -not $sp.snapshot) { return $false }
    if ($UserSid -notmatch '^S-1-5-21(-\d+)+$') { return $false }
    $key = "Registry::HKEY_USERS\$UserSid\Software\Microsoft\Windows\CurrentVersion\Internet Settings"
    if (-not (Test-Path -LiteralPath $key)) { return $false } # user logged off: the hive is not loaded
    $cur = Get-ItemProperty -LiteralPath $key
    if ($cur.ProxyEnable -ne 1 -or $cur.ProxyServer -ne $sp.ours) { return $false }
    try {
        $snap = $sp.snapshot
        $flags = [uint32]$snap.flags
        Set-ItemProperty -LiteralPath $key -Name ProxyEnable -Value ([int](($flags -band 2) -ne 0)) -Type DWord
        Set-ItemProperty -LiteralPath $key -Name ProxyServer -Value ([string]$snap.server)
        Set-ItemProperty -LiteralPath $key -Name ProxyOverride -Value ([string]$snap.bypass)
        if (($flags -band 4) -ne 0 -and $snap.autoconfigUrl) {
            Set-ItemProperty -LiteralPath $key -Name AutoConfigURL -Value ([string]$snap.autoconfigUrl)
        } else {
            Remove-ItemProperty -LiteralPath $key -Name AutoConfigURL -ErrorAction SilentlyContinue
        }
        $conn = "$key\Connections"
        $blob = (Get-ItemProperty -LiteralPath $conn -ErrorAction SilentlyContinue).DefaultConnectionSettings
        if ($blob -and $blob.Length -ge 24) {
            $new = Set-ConnectionBlob $blob $flags $snap.server $snap.bypass $snap.autoconfigUrl
            Set-ItemProperty -LiteralPath $conn -Name DefaultConnectionSettings -Value $new -Type Binary
        }
        Write-GuardLog "restored system proxy (was $($sp.ours))"
        return $false
    } catch {
        Write-GuardLog "restore system proxy failed: $_"
        return $true
    }
}

# state.json is gone (a portable folder deleted with the exe): no snapshot
# to restore. Like sysdns.LoopbackAdapters, a family whose DNS is exactly
# loopback is Ghostline's fingerprint; it goes back to DHCP. Returns how
# many are still stuck there.
function Reset-LoopbackDNS {
    $stuck = 0
    foreach ($f in @(@{ Name = 'ipv4'; Family = 'IPv4'; Loop = '127.0.0.1' }, @{ Name = 'ipv6'; Family = 'IPv6'; Loop = '::1' })) {
        foreach ($d in @(Get-DnsClientServerAddress -AddressFamily $f.Family -ErrorAction SilentlyContinue)) {
            $cur = @($d.ServerAddresses)
            if ($cur.Count -ne 1 -or $cur[0] -ne $f.Loop) { continue }
            try {
                Set-FamilyDNS $d.InterfaceIndex $f.Name @()
                Write-GuardLog "no state: reset $($f.Name) DNS on '$($d.InterfaceAlias)' from $($f.Loop) to DHCP"
            } catch {
                Write-GuardLog "no state: DHCP on '$($d.InterfaceAlias)' failed: $_"
                $stuck++
            }
        }
    }
    try { Clear-DnsClientCache } catch { }
    return $stuck
}

# state.json is gone: without Ghostline's address, a proxy on loopback that
# nothing listens on is the fingerprint. It is switched off, which is what
# almost every snapshot holds.
function Reset-DeadLoopbackProxy {
    if ($UserSid -notmatch '^S-1-5-21(-\d+)+$') { return }
    $key = "Registry::HKEY_USERS\$UserSid\Software\Microsoft\Windows\CurrentVersion\Internet Settings"
    if (-not (Test-Path -LiteralPath $key)) { return }
    $cur = Get-ItemProperty -LiteralPath $key
    if ($cur.ProxyEnable -ne 1 -or [string]$cur.ProxyServer -notmatch '^127\.0\.0\.1:(\d+)$') { return }
    if (Get-NetTCPConnection -State Listen -LocalPort ([int]$Matches[1]) -ErrorAction SilentlyContinue) { return }
    try {
        Set-ItemProperty -LiteralPath $key -Name ProxyEnable -Value 0 -Type DWord
        $conn = "$key\Connections"
        $blob = (Get-ItemProperty -LiteralPath $conn -ErrorAction SilentlyContinue).DefaultConnectionSettings
        if ($blob -and $blob.Length -ge 24) {
            $flags = [BitConverter]::ToUInt32($blob, 8) -band (-bnot [uint32]2)
            $new = Set-ConnectionBlob $blob $flags '' '' ''
            Set-ItemProperty -LiteralPath $conn -Name DefaultConnectionSettings -Value $new -Type Binary
        }
        Write-GuardLog "no state: switched off dead proxy $($cur.ProxyServer)"
    } catch {
        Write-GuardLog "no state: switching off proxy failed: $_"
    }
}

try {
    if (-not (Test-Path -LiteralPath $State)) {
        # Ghostline still serving DNS (its folder moved while connected): leave it.
        if (Get-NetUDPEndpoint -LocalAddress 127.0.0.1 -LocalPort 53 -ErrorAction SilentlyContinue) { return }
        Write-GuardLog "state.json is gone ($State); resetting loopback DNS to DHCP"
        $stuck = Reset-LoopbackDNS
        Reset-DeadLoopbackProxy
        if ($stuck -eq 0) { Remove-Guard 'no state' }
        return
    }
    # Mitigate LPE: Ensure state.json is authored by Administrators or SYSTEM.
    try {
        $owner = (Get-Acl -LiteralPath $State).Owner
        $sid = (New-Object System.Security.Principal.NTAccount($owner)).Translate([System.Security.Principal.SecurityIdentifier]).Value
        if ($sid -ne 'S-1-5-18' -and $sid -ne 'S-1-5-32-544') {
            Write-GuardLog "state.json owner ($owner, $sid) is untrusted; resetting to DHCP to prevent LPE"
            Reset-LoopbackDNS
            Reset-DeadLoopbackProxy
            Remove-Guard 'untrusted state owner'
            return
        }
    } catch {
        Write-GuardLog "verifying state.json owner failed: $_"
        return
    }
    try {
        $st = Get-Content -LiteralPath $State -Raw -Encoding UTF8 | ConvertFrom-Json
    } catch {
        Write-GuardLog "state.json unreadable: $_" # mid-write or corrupt: try again next run
        return
    }
    if ($st.phase -ne 'dns_set') { Remove-Guard 'state clean'; return }
    if (Test-GhostlineAlive $st.pid $st.pidStartTime) { return }
    Write-GuardLog "Ghostline (pid $($st.pid)) is gone without restoring; checking DNS and proxy"
    $stuck = Restore-DNS $st.snapshot
    $proxyStuck = Restore-Proxy $st.sysproxy
    if ($stuck -eq 0 -and -not $proxyStuck) { Remove-Guard 'restored' }
} catch {
    Write-GuardLog "guard failed: $_"
}
