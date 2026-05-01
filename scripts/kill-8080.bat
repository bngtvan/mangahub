@echo off
powershell -ExecutionPolicy Bypass -File "%~dp0kill-port.ps1" -Port 8080
