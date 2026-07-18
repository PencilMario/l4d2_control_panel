.PHONY: test build compose-check deploy-test
test:
	go test ./...
	cd web && npm test -- --run
	bash -n deploy.sh
	bash deploy_test.sh
build:
	go build ./cmd/panel
	go build ./cmd/overlay-helper
	cd web && npm run build
compose-check:
	docker compose --env-file .env.example config --quiet
deploy-test:
	bash -n deploy.sh
	bash deploy_test.sh
