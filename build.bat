@echo off
go build -ldflags "-s -w" -o go_wrapper.exe main.go
where upx >nul 2>nul && upx -9 go_wrapper.exe
