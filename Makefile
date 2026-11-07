.PHONY: api web test build
api:
	cd backend && go run ./cmd/server
web:
	cd frontend && npm run dev
test:
	cd backend && go test -race ./...
	cd frontend && npm run typecheck
build:
	cd backend && go build -o /tmp/alisa-portal-api ./cmd/server
	cd frontend && npm run build
