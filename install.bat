@echo off
chcp 65001 >nul
cd /d "%~dp0"

set "LOADER=%~dp0ImNoDemonlord-ModLoader.exe"
if not exist "%LOADER%" set "LOADER=%~dp0dist\ImNoDemonlord-ModLoader.exe"
if not exist "%LOADER%" set "LOADER=%~dp0loader\imnd-loader.exe"

if not exist "%LOADER%" goto noloader

echo ============================================================
echo  Im No Demonlord - offline install (patches app.asar)
echo ============================================================
echo.
"%LOADER%" install %*
echo.
pause
exit /b 0

:noloader
echo [ERROR] ImNoDemonlord-ModLoader.exe was not found.
echo Put this file next to the loader exe, or run build.bat first.
pause
exit /b 1
