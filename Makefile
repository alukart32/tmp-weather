.PHONY: help
help:
	@echo List of commands:
	@echo   unit-test               - run unit-tests
	@echo   integration-test        - run integration-tests
	@echo   docker-up               - docker compose up
	@echo   docker-down             - docker compose down
	@echo Usage:
	@echo                           make `cmd_name`

.PHONY: unit-test
unit-test:
	./scripts/test.sh tmpweather short .env

.PHONY: integration-test
integration-test:
	@./scripts/test.sh tmpweather integration .test.env

.PHONY: docker-up
docker-up:
	@./scripts/dockerup.sh dev

.PHONY: docker-down
docker-down:
	@./scripts/dockerdown.sh dev
