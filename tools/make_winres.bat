@echo off
setlocal
cd /d "%~dp0.."

echo [1/2] Drawing the icon ...
go run tools\icongen\main.go
if errorlevel 1 goto fail

echo [2/2] Building the Windows resources ...
go-winres simply --arch amd64 --manifest gui --icon assets/icon.ico --out rsrc --product-version 1.0.0.0 --file-version 1.0.0.0 --product-name "ImNoDemonlord Mod Loader" --file-description "Im No Demonlord - cheat mod loader" --copyright "For single-player use only" --original-filename "ImNoDemonlord-ModLoader.exe"
if errorlevel 1 goto fail

echo Done: loader\rsrc_windows_amd64.syso
exit /b 0

:fail
echo.
echo Failed. If go-winres is missing, install it with:
echo   go install github.com/tc-hib/go-winres@latest
exit /b 1
