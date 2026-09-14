@echo off
setlocal

rem Use UTF-8 console codepage so the demo's Chinese output renders correctly.
chcp 65001 >nul

rem Switch to the script directory so relative "go run" paths work.
cd /d "%~dp0"

rem Connection settings: environment variables take precedence over defaults.
if "%WAYMARK_ENDPOINT%"=="" set WAYMARK_ENDPOINT=http://127.0.0.1:9868
if "%WAYMARK_USERNAME%"=="" set WAYMARK_USERNAME=admin
if "%WAYMARK_PASSWORD%"=="" set WAYMARK_PASSWORD=123456
if "%WAYMARK_NAMESPACE%"=="" set WAYMARK_NAMESPACE=public

echo ============================================================
echo  Combined demo (ConfigWatcher + RegistryResolver)
echo  endpoint  = %WAYMARK_ENDPOINT%
echo  username  = %WAYMARK_USERNAME%
echo  namespace = %WAYMARK_NAMESPACE%
echo ============================================================
echo.

go run ./cmd/all %*
set EXIT_CODE=%ERRORLEVEL%

echo.
echo ==== exited with code %EXIT_CODE% ====
pause
exit /b %EXIT_CODE%
