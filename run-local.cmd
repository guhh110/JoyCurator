@echo off
setlocal
set "ROOT=%~dp0"
set "LOCAL=%ROOT%local"
set "GOROOT=%LOCAL%\tools\go"
set "GOPATH=%LOCAL%\cache\gopath"
set "GOMODCACHE=%LOCAL%\cache\go-mod"
set "GOCACHE=%LOCAL%\cache\go-build"
set "GOTMPDIR=%LOCAL%\tmp"
set "GOENV=off"
set "GOTOOLCHAIN=local"
set "GOPROXY=https://goproxy.cn"
set "TEMP=%LOCAL%\tmp"
set "TMP=%LOCAL%\tmp"
set "TMPDIR=%LOCAL%\tmp"
set "npm_config_cache=%LOCAL%\cache\npm"
set "PNPM_HOME=%LOCAL%\tools\pnpm"
set "PATH=%LOCAL%\tools\node;%GOROOT%\bin;%PATH%"
if not exist "%LOCAL%\tmp" mkdir "%LOCAL%\tmp"
if not exist "%LOCAL%\logs" mkdir "%LOCAL%\logs"
if not exist "%LOCAL%\data" mkdir "%LOCAL%\data"
if not exist "%LOCAL%\source" mkdir "%LOCAL%\source"
if not exist "%LOCAL%\target" mkdir "%LOCAL%\target"
if /i "%~1"=="backend" goto backend
if /i "%~1"=="frontend" goto frontend
if not "%~1"=="" (
  echo Usage: run-local.cmd [backend^|frontend]
  exit /b 2
)
echo JoyCurator: backend http://127.0.0.1:8081, frontend http://127.0.0.1:5175
start "JoyCurator Backend" cmd /k ""%~f0" backend"
start "JoyCurator Frontend" cmd /k ""%~f0" frontend"
exit /b 0
:backend
cd /d "%ROOT%backend" || exit /b 1
"%GOROOT%\bin\go.exe" run ./cmd/joycurator
exit /b %errorlevel%
:frontend
cd /d "%ROOT%frontend" || exit /b 1
"%LOCAL%\tools\node\node.exe" "%LOCAL%\tools\pnpm\bin\pnpm.mjs" dev
exit /b %errorlevel%
