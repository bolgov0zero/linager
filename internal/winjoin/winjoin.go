// Package winjoin builds the PowerShell that installs the Windows agent by a link: the one-line command a person pastes
// and the script it fetches. It has no dependencies, so it can be tested on Windows itself.
package winjoin

import (
	"net"
	"strings"
)

// psq quotes a value for a PowerShell single-quoted string.
func psq(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// pinCode is the C# that makes PowerShell accept the certificate of this server and no other: the fingerprint of the
// certificate (the same one the agent pins) is compared with the one the command carries.
const pinCode = `using System;using System.Net.Security;using System.Security.Cryptography;using System.Security.Cryptography.X509Certificates;` +
	`public class LinagerPin{public static string Fp;public static RemoteCertificateValidationCallback Callback=new RemoteCertificateValidationCallback(Check);` +
	`static bool Check(object s,X509Certificate c,X509Chain h,SslPolicyErrors e){if(c==null)return false;using(var sha=SHA256.Create()){return "SHA256:"+Convert.ToBase64String(sha.ComputeHash(c.GetRawCertData())).TrimEnd('=')==Fp;}}}`

// PinSetup is the PowerShell that turns the pin on; the command and the script both use it.
func PinSetup(fp string) string {
	return "$ErrorActionPreference='Stop'; try { [Net.ServicePointManager]::SecurityProtocol = 3072 -bor 12288 } catch { [Net.ServicePointManager]::SecurityProtocol = 3072 }; " +
		"if (-not ('LinagerPin' -as [type])) { Add-Type -TypeDefinition " + psq(pinCode) + " }; [LinagerPin]::Fp=" + psq(fp) +
		"; [Net.ServicePointManager]::ServerCertificateValidationCallback=[LinagerPin]::Callback"
}

// InstallCommand is the line a person runs in PowerShell as administrator on the new computer.
func InstallCommand(host, port, pinFp, secret string) string {
	// the filter of certificates is switched off again at the end: it would refuse every other HTTPS site in this window
	return PinSetup(pinFp) + "; try { iex ((New-Object Net.WebClient).DownloadString(" + psq("https://"+net.JoinHostPort(host, port)+"/v1/join/"+secret+"/windows") + ")) } finally { [Net.ServicePointManager]::ServerCertificateValidationCallback = $null }"
}

func ScriptText(server, fp, pinFp, token, msiSHA256 string) string {
	return `# Linager: install the Windows agent by a link made in the application.
# The script comes from your Linager server; read it before you run it.
` + PinSetup(pinFp) + `
$server = ` + psq(server) + `
$fp = ` + psq(fp) + `
$token = ` + psq(token) + `
$want = ` + psq(strings.ToUpper(msiSHA256)) + `

$me = [Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
if (-not $me.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Run PowerShell as administrator.' }
if (-not [Environment]::Is64BitOperatingSystem) { throw 'The agent needs 64-bit Windows.' }

$msi = Join-Path $env:TEMP 'linager-agent_windows_amd64.msi'
"Downloading the agent from $server ..."
(New-Object Net.WebClient).DownloadFile("https://$server/v1/join/$token/msi", $msi)
if ((Get-FileHash -Algorithm SHA256 -LiteralPath $msi).Hash -ne $want) { Remove-Item -LiteralPath $msi -Force; throw 'The installer does not match the signed release.' }

# The one-time code of the link goes to the service through a file that only SYSTEM and the administrators can read.
$dir = Join-Path $env:ProgramData 'Linager\agent'
New-Item -ItemType Directory -Force -Path $dir | Out-Null
& icacls.exe (Split-Path $dir) /inheritance:r /grant:r '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' | Out-Null
Set-Content -LiteralPath (Join-Path $dir 'join.token') -Value $token -NoNewline -Encoding ASCII

# the certificate filter is not needed any more (the installer comes from this server only)
[Net.ServicePointManager]::ServerCertificateValidationCallback = $null
"Installing ..."
$p = Start-Process -FilePath msiexec.exe -ArgumentList ('/i "' + $msi + '" /qn SERVER=' + $server + ' FINGERPRINT=' + $fp) -Wait -PassThru
Remove-Item -LiteralPath $msi -Force -ErrorAction SilentlyContinue
if ($p.ExitCode -ne 0) { throw "The installation failed (code $($p.ExitCode))." }
'Done. The computer appears in the panel: Add server, tab Confirmations. Confirm it there.'
`
}
