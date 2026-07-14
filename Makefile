.PHONY: test build compose-check
test:
	go test ./...
	cd web && npm test -- --run
build:
	go build ./cmd/panel
	cd web && npm run build
compose-check:
	docker compose --env-file .env.example config --quiet
