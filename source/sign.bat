@echo off
rem Optional helper - signs SWTLUpdater.exe with YOUR code-signing certificate (untested here: needs your certificate).
rem Needs signtool.exe from the Windows SDK. Usage:  sign.bat  path\to\certificate.pfx  certificate-password
rem (If your certificate is on a hardware token or cloud HSM, use the signing command your provider gives you instead.)
if "%~2"=="" (echo Usage: sign.bat certificate.pfx password & exit /b 1)
signtool sign /fd SHA256 /f "%~1" /p "%~2" /tr http://timestamp.digicert.com /td SHA256 /d "SW TL Updater" /du "https://discord.com" "%~dp0SWTLUpdater.exe"
signtool verify /pa "%~dp0SWTLUpdater.exe"
