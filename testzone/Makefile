# Isolated FidoNet test zone.
.PHONY: up down restart rebuild logs mailer-log shell test clean

up:            ## Start the zone (builds on first run)
	docker compose up -d --build
	@echo "waiting for the mailer to come up..."
	@for i in $$(seq 1 40); do \
	    docker compose logs 2>/dev/null | grep -q "ready -- mailer log follows" && break; \
	    sleep 1; \
	done
	@docker compose logs 2>/dev/null | grep -E "listening for|system open" || true

down:          ## Stop the zone, keeping MBSE's databases
	docker compose down

restart:
	docker compose restart

rebuild:       ## Rebuild the image from scratch
	docker compose build --no-cache
	docker compose up -d

logs:          ## Container stdout (boot messages + live mailer log)
	docker compose logs -f

mailer-log:    ## What the remote sysop would read
	docker exec testzone-mbse tail -n 60 /opt/mbse/log/system.log

debug-log:     ## Protocol-level trace (binkp state machine, ZMODEM headers)
	docker exec testzone-mbse tail -n 80 /opt/mbse/log/debug.log

shell:
	docker exec -it testzone-mbse bash

test:          ## Drive the local testdaemon against the zone
	@test -n "$(CONFIG)" || { echo "usage: make test CONFIG=/path/to/testdaemon.yaml"; exit 2; }
	CONFIG=$(CONFIG) ./run-tests.sh

clean:         ## Remove the zone and MBSE's databases
	docker compose down -v
