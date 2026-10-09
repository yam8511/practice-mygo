# Builds my-app with go build instead of mygo build, so GoCV can use cgo,
# and links in what mygo build would: the app's name and version from
# mygo.json, and the update feed and key, so the app checks for updates.
#
# Unlike mygo build, it makes no installer, update archive or delta, and the
# executable has no icon: publish updates with mygo build.
#
#   .\build-setup.ps1              # build\cgo-windows-amd64\my-app.exe
#   .\build-setup.ps1 -Out dist

param(
    [string]$Out = "build\cgo-windows-amd64",
    [string]$OpenCV = "C:\opencv"
)
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

$cfg = Get-Content -Raw mygo.json | ConvertFrom-Json
if (-not $cfg.updates.url -or -not $cfg.updates.publicKey) {
    throw "mygo.json needs updates.url and updates.publicKey"
}
$feed = $cfg.updates.url.TrimEnd("/") + "/update-windows-amd64.json"

# GoCV against OpenCV 4.13.0, as build-camera.ps1 builds it.
$cv = $OpenCV -replace "\\", "/"
$env:CGO_ENABLED = "1"
$env:CGO_CXXFLAGS = "--std=c++11"
$env:CGO_CPPFLAGS = "-I$cv/include"
$libs = "core","face","videoio","imgproc","highgui","imgcodecs","objdetect","features2d","video","dnn","xfeatures2d","plot","tracking","img_hash","calib3d","photo","aruco","wechat_qrcode","ximgproc","xphoto","bgsegm","bioinspired","ccalib"
$env:CGO_LDFLAGS = "-L$cv/x64/mingw/lib " + (($libs | % { "-lopencv_${_}4130" }) -join " ")
$bin = Join-Path $OpenCV "x64\mingw\bin"
$env:Path = "$bin;$env:Path"
$env:GOOS = "windows"
$env:GOARCH = "amd64"

# The variables mygo build sets (packageFlags and updateFlags in
# cmd/mygo): without production=1 the app counts as a development build,
# and without the feed and key its updater is disabled.
$m = "github.com/egoist/mygo"
$ld = @(
    "-s", "-w", "-H=windowsgui",
    "-X", "$m.production=1",
    "-X", "$m.packageName=$($cfg.name)",
    "-X", "$m.packageVersion=$($cfg.version)",
    "-X", "$m.packageUpdateFeed=$feed",
    "-X", "$m.packageUpdateKey=$($cfg.updates.publicKey)"
)
if ($cfg.identifier) { $ld += "-X", "$m.packageIdentifier=$($cfg.identifier)" }

New-Item -ItemType Directory -Force $Out | Out-Null
$exe = Join-Path $Out "$($cfg.name).exe"
Write-Host "building $exe $($cfg.version), updates from $feed"
go build -trimpath -tags mygo_noinspector -ldflags ($ld -join " ") -o $exe .
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

# Next to the executable, as mygo build puts them: the resources for all
# platforms and for Windows, and the DLLs of OpenCV and MinGW it runs with.
if (Test-Path resources) {
    Get-ChildItem resources -File | Copy-Item -Destination $Out -Force
    foreach ($dir in "windows", "windows-amd64") {
        if (Test-Path "resources\$dir") { Copy-Item "resources\$dir\*" $Out -Recurse -Force }
    }
}
# Copy-Item "$bin\*.dll" $Out -Force
Write-Host "built $exe"
