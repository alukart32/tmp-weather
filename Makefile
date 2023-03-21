pg_name = postgres15
pg_user = postgres
pg_user_pass = postgres
pg_image = postgres:15-alpine
pg_uri = localhost:5432
db_name = tmpweather

.PHONY: help
help:
	@echo List of params:
	@echo   db_name                 - postgres docker container name (default: $(pg_name))
	@echo   pq_user                 - postgres root user (default: $(pg_user))
	@echo   pq_user_pass            - postgres root user password (default: $(pg_user_pass))
	@echo   db_image                - postgres docker image (default: $(pg_image))
	@echo   db_uri                  - postgres uri (default: $(pg_uri))
	@echo   db_name                 - postgres main db (default: $(db_name))
	@echo List of commands:
	@echo   pq-up                   - run postgres postgres docker container $(pg_name)
	@echo   pq-up                   - down postgres postgres docker container $(pg_name)
	@echo   create-db               - create db $(db_name)
	@echo   drop-db                 - drop db $(db_name)
	@echo   test                    - run all tests
	@echo   help                    - help info
	@echo Usage:
	@echo                           make `cmd_name`

.PHONY: pq-up
pq-up:
	docker run --name $(pg_name) -e POSTGRES_USER=$(pg_user) -e POSTGRES_PASSWORD=$(pg_user_pass) -p 5432:5432 -d $(pg_image)

.PHONY: pq-stop
pq-stop:
	docker stop $(pg_name)

.PHONY: create-db
create-db:
	docker exec -it $(pg_name) createdb --username=$(pg_user) --owner=$(pg_user) $(db_name)

.PHONY: drop-db
drop-db:
	docker exec -it $(pg_name) dropdb --username=$(pg_user) $(db_name)

.PHONY: test
test:
	go test -cover ./...
