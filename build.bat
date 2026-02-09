@echo off
go build -ldflags "-s -w" -o go_wrapper.exe main.go