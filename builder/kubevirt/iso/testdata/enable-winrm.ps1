Enable-PSRemoting -Force
Enable-WSManCredSSP -Role Server -Force
Set-Item -Path WSMan:\localhost\Service\Auth\Basic -Value $true
Set-Item -Path WSMan:\localhost\Service\AllowUnencrypted -Value $true
New-NetFirewallRule -Name "WinRM_HTTP" -DisplayName "WinRM over HTTP" -Protocol TCP -LocalPort 5985 -Action Allow
