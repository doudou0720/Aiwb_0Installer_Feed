#!/bin/bash
go build -ldflags "-s -w" -o go_wrapper main.go
command -v upx &> /dev/null && upx -9 go_wrapper
