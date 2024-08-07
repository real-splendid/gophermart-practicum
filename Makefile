generate-mocks:
	mockgen -source=internal/storage/storage.go -destination=internal/storage/storage_mock.go -package=storage Storage

migrate:
	go run github.com/pressly/goose/v3/cmd/goose -dir migrations/ postgres postgresql://app:pass@127.0.0.1:5432/gophermart?sslmode=disable up

migrate-down:
	go run github.com/pressly/goose/v3/cmd/goose -dir migrations/ postgres postgresql://app:pass@127.0.0.1:5432/gophermart?sslmode=disable down

statictest:
	go vet -vettool=cmd/statictest/statictest  ./...

gophermarttest:
	go build -o cmd/gophermart/gophermart cmd/gophermart/main.go
	cmd/gophermarttest/gophermarttest \
		-test.v \
		-test.run=^TestGophermart \
		-gophermart-binary-path=cmd/gophermart/gophermart \
		-gophermart-host=localhost \
		-gophermart-port=8080 \
		-gophermart-database-uri="postgresql://app:pass@127.0.0.1:5432/gophermart?sslmode=disable" \
		-accrual-binary-path=cmd/accrual/accrual_linux_amd64 \
		-accrual-host=localhost \
		-accrual-port=8081 \
		-accrual-database-uri="postgresql://app:pass@127.0.0.1:5433/gophermart?sslmode=disable"

up:
	docker compose up -d

down:
	docker compose down
