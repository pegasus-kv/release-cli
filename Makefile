build:
	go mod tidy
	go mod verify
	go build -o ./release-cli

