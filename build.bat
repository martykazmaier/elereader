@echo off
setlocal
set GOOS=windows
set GOARCH=386
set CGO_ENABLED=0
go build -ldflags "-s -w" -o elereader.exe .
if errorlevel 1 exit /b 1
echo Built elereader.exe (windows/386)
