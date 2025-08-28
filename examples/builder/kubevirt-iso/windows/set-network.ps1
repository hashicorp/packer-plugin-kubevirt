# Get current network
$profile = Get-NetConnectionProfile

# Set network to Private from Public (required to enable WinRM)
Set-NetConnectionProfile -InterfaceIndex $profile.InterfaceIndex -NetworkCategory Private
