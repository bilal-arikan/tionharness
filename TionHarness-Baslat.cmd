@echo off
REM ============================================================
REM  TionHarness baslatici - cift tikla calistir.
REM  Backend'i 127.0.0.1:5174'te baslatir - tailscale serve bu porta
REM  proxy'ledigi icin baska bir sey gerekmez.
REM  Telefon (Tailscale acik): https://desktop-obbv4nm.tailf9c4c9.ts.net/
REM  Not: exe'yi scripts\build.ps1 kok dizine uretir.
REM ============================================================
set TIONHARNESS_ADDR=127.0.0.1:5174
if not exist "%~dp0tionharness.exe" (
  echo HATA: tionharness.exe yok. Once derle:  .\scripts\build.ps1
  pause
  exit /b 1
)
start "" "%~dp0tionharness.exe"
echo.
echo TionHarness baslatildi ^(127.0.0.1:5174^).
echo Telefondan ac ^(Tailscale acik^):
echo   https://desktop-obbv4nm.tailf9c4c9.ts.net/
echo.
REM timeout yerine ping: stdin yonlendirilmis ortamda (script/CI) timeout hata verir.
ping -n 5 127.0.0.1 >nul
