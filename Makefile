build_part:
	echo "Compiling part execution for every OS and Platform"
	GOOS=linux GOARCH=amd64 go build -o part_linux part.go
	GOOS=freebsd GOARCH=amd64 go build -o part_bsd part.go
	GOOS=windows GOARCH=amd64 go build -o part_win.exe part.go
	GOOS=darwin GOARCH=amd64 go build -o part_darwin part.go

all: build_part
