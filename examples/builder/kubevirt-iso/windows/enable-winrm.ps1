# Enable WinRM
Enable-PSRemoting -Force

# Allow remote access to WinRM
Enable-WSManCredSSP -Role Server -Force

# Allow basic authentication
Set-Item -Path WSMan:\localhost\Service\Auth\Basic -Value $true

# Allow unencrypted traffic on the server
Set-Item -Path WSMan:\localhost\Service\AllowUnencrypted -Value $true

# Enables HTTP listener on port 5985
New-NetFirewallRule -Name "WinRM_HTTP" -DisplayName "WinRM over HTTP" `
  -Protocol TCP -LocalPort 5985 -Action Allow
