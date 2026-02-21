@echo off
setlocal EnableExtensions

set "ROOT=%~dp0.."
for %%I in ("%ROOT%") do set "ROOT=%%~fI"
set "OUT=%ROOT%\build\windows"

if not exist "%OUT%" mkdir "%OUT%"

echo [SIDE] Building Windows binary...
cd /d "%ROOT%"
go build -trimpath -ldflags="-s -w" -o "%OUT%\side.exe" ./cmd/side
if errorlevel 1 (
    echo [SIDE] Build failed.
    exit /b 1
)

echo [SIDE] Build completed.
echo [SIDE] Output: %OUT%\side.exe
