@echo off
REM ============================================================
REM  TionSwarm baslatici - cift tikla calistir.
REM  Backend'i 127.0.0.1:8080'de baslatir. Tailscale serve zaten
REM  kalici oldugu icin baska bir sey gerekmez.
REM  Telefon (Tailscale acik): https://desktop-obbv4nm.tailf9c4c9.ts.net/
REM ============================================================
set TIONSWARM_ADDR=127.0.0.1:5174
start "" "%~dp0bin\tionswarm.exe"
echo.
echo TionSwarm baslatildi ^(127.0.0.1:8080^).
echo Telefondan ac ^(Tailscale acik^):
echo   https://desktop-obbv4nm.tailf9c4c9.ts.net/
echo.
timeout /t 4 >nul
