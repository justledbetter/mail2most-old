all:
	go build

install:
	go install

clean:
	go clean
	rm -f mail2most
