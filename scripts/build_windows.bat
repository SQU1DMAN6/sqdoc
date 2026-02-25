@echo off
setlocal EnableExtensions

set "ROOT=%~dp0.."
for %%I in ("%ROOT%") do set "ROOT=%%~fI"
set "OUT=%ROOT%\build\windows"
set "META=%OUT%\BUILD\Meta.config"
set "WXS=%OUT%\side.wxs"
set "VERSION=0.0.0"

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

if exist "%META%" (
    for /f "usebackq tokens=1,* delims==" %%A in ("%META%") do (
        if /I "%%A"=="VERSION" set "VERSION=%%B"
    )
)

if not exist "%WXS%" (
    echo [SIDE] MSI source not found: %WXS%
    exit /b 0
)

where wix >nul 2>nul
if errorlevel 1 (
    echo [SIDE] WiX toolset not found. Skipping MSI build.
    echo [SIDE] Install WiX v4 and ensure ^`wix^` is in PATH to build MSI.
    exit /b 0
)

set "MSI_OUT=%OUT%\SIDE-%VERSION%-win-x64.msi"
echo [SIDE] Building MSI installer...
pushd "%OUT%"
wix build "side.wxs" -arch x64 -out "%MSI_OUT%"
set "WIX_EXIT=%ERRORLEVEL%"
popd
if not "%WIX_EXIT%"=="0" (
    echo [SIDE] MSI build failed.
    exit /b 1
)
echo [SIDE] MSI output: %MSI_OUT%
