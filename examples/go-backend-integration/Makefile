.PHONY: test run compose-up compose-down send-bug send-feedback

test:
	go test ./...

run:
	go run ./cmd/server

compose-up:
	docker compose up --build

compose-down:
	docker compose down

send-bug:
	curl --fail-with-body --request POST http://localhost:8081/v1/support-submissions \
		--header 'Authorization: Bearer local-example-backend-token-change-me' \
		--header 'Content-Type: application/json' \
		--header 'Idempotency-Key: example-bug-submission-0001' \
		--header "X-External-Request-ID: req_$$(openssl rand -hex 16)" \
		--data @requests/bug.json

send-feedback:
	curl --fail-with-body --request POST http://localhost:8081/v1/support-submissions \
		--header 'Authorization: Bearer local-example-backend-token-change-me' \
		--header 'Content-Type: application/json' \
		--header 'Idempotency-Key: example-feedback-submission-0001' \
		--header "X-External-Request-ID: req_$$(openssl rand -hex 16)" \
		--data @requests/feedback.json
