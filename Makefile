.PHONY: build release dev-backend dev-frontend run deploy provision

# VPS architecture: amd64 or arm64
GOARCH ?= amd64

build:            ## build frontend + local binary ./cheatchess
	cd frontend && npm install && npm run build
	cd backend && go build -trimpath -ldflags="-s -w" -o ../cheatchess .

release:          ## static Linux binary for the VPS -> dist/cheatchess
	cd frontend && npm ci && npm run build
	cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH) go build -trimpath -ldflags="-s -w" -o ../dist/cheatchess .

provision:        ## one-time VPS setup (Caddy, Stockfish, systemd, firewall)
	cd deploy/ansible && ansible-playbook provision.yml

deploy: release   ## build + deploy from this machine
	cd deploy/ansible && ansible-playbook deploy.yml

dev-backend:      ## API + websocket on :8080
	cd backend && go run .

dev-frontend:     ## Vite dev server with hot reload on :5173 (proxies to :8080)
	cd frontend && npm run dev

run: build
	./cheatchess
