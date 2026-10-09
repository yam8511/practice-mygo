$env:AWS_ACCESS_KEY_ID="admin"
$env:AWS_SECRET_ACCESS_KEY="admin"
$env:MYGO_UPDATER_PRIVATE_KEY=Get-Content -Raw "$env:APPDATA\mygo\update-keys\mygo-update.key"
mygo build
