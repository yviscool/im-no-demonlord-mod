@echo off
setlocal
cd /d "%~dp0"

where go >nul 2>nul
if errorlevel 1 goto nogo

echo [1/3] Syncing cheat.js ...
copy /Y "src\cheat.js" "loader\assets\cheat.js" >nul
if errorlevel 1 goto fail

echo [2/3] Building the loader ...
if not exist "dist" mkdir "dist"
pushd loader
go build -trimpath -ldflags "-s -w -H=windowsgui" -o "..\dist\ImNoDemonlord-ModLoader.exe" .
set ERR=%errorlevel%
popd
if not "%ERR%"=="0" goto fail

echo [3/3] Packing the release zip ...
go run tools\package\main.go
if errorlevel 1 goto fail

echo.
echo Done. See the files in dist\.
exit /b 0

:nogo
echo [ERROR] Go was not found. Install Go 1.27.3 or newer.
pause
exit /b 1

:fail
echo [ERROR] Build failed.
pause
exit /b 1
