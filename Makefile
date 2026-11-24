.PHONY: test race demo

test:
	go test ./...

race:
	go test -race -count=1 ./...

demo:
	go run ./cmd/demo
