PKGS = $(shell go list ./... | grep -v /vendor/ | grep -v /bindata)

cover:
	@go test -coverprofile=cover.out -coverpkg=./... ./...
	@go tool cover -html=cover.out

count:
	@find \
		./pkg/agl \
		./scripts \
		-name '*.go' \
		| xargs wc -l \
		| sort