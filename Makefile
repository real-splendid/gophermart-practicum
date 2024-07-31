statictest:
	go vet -vettool=cmd/statictest/statictest  ./...

gophermarttest:
	cmd/gophermarttest/gophermarttest \
		-test.v -test.run=^TestGophermart$ \
		-gophermart-binary-path=cmd/gophermart/gophermart \
		-gophermart-host=localhost \
		-gophermart-port=8080 \
		-gophermart-database-uri="postgresql://postgres:pass@db/gophermart?sslmode=disable" \
		-accrual-binary-path=cmd/accrual/accrual_linux_amd64 \
		-accrual-host=localhost \
		-accrual-port=8081 \
		-accrual-database-uri="postgresql://postgres:pass@db/gophermart?sslmode=disable"

up:
	docker compose up -d

down:
	docker compose down
