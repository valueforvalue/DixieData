@echo off
cd /d C:\Development\DixieData\tools\tune
del bin\dixiedata-tune.exe 2>nul
del bin\dixiedata-tune 2>nul
go build -o bin\dixiedata-tune.exe .
echo build exit %ERRORLEVEL%
dir bin\
