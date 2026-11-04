@echo off
chcp 65001 >nul
cd /d "%~dp0"

where go >nul 2>nul
if errorlevel 1 goto nogo

echo [1/3] Syncing cheat.js ...
copy /Y "src\cheat.js" "loader\assets\cheat.js" >nul
if errorlevel 1 goto nocopy

if not exist "dist" mkdir "dist"

echo [2/3] Building the loader ...
pushd loader
go build -trimpath -ldflags "-s -w -H=windowsgui" -o "..\dist\ImNoDemonlord-ModLoader.exe" .
set ERR=%errorlevel%
popd
if not "%ERR%"=="0" goto buildfail

echo [3/3] Done: dist\ImNoDemonlord-ModLoader.exe
echo Double-click it, or run start-loader.bat.
echo.
pause
exit /b 0

:nogo
echo [ERROR] Go was not found. Install Go 1.27.3 or newer.
pause
exit /b 1

:nocopy
echo [ERROR] Could not copy src\cheat.js.
pause
exit /b 1

:buildfail
echo [ERROR] Build failed.
pause
exit /b 1
