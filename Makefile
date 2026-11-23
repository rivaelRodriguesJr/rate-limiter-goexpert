.PHONY: run test down

run:
	docker compose up --build

test:
	docker compose --profile test run --rm test

down:
	docker compose --profile test down -v
