@echo off
powershell -NoProfile -Command "Start-Process pwsh -Verb RunAs -ArgumentList '-NoExit','-NoProfile','-ExecutionPolicy','Bypass','-File','\"%~dp0realnet_check.ps1\"'"
