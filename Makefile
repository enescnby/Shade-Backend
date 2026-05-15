# ══════════════════════════════════════════════════════════════════
#  SHADE BACKEND — Makefile
#  Kullanım: make <hedef>
#  Tüm hedefler: make help
# ══════════════════════════════════════════════════════════════════

BINARY_NAME = shade-backend
MAIN_PATH   = ./cmd/api
BUILD_DIR   = ./bin

.PHONY: help dev build run test lint security clean \
        docker-up docker-down docker-logs migrate-up migrate-down \
        setup check

# ── Varsayılan hedef ─────────────────────────────────────────────
.DEFAULT_GOAL := help

help: ## Kullanılabilir komutları göster
	@echo ""
	@echo "  SHADE Backend — Geliştirici Komutları"
	@echo "  ──────────────────────────────────────"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""

# ── Geliştirme ───────────────────────────────────────────────────
dev: ## Hot-reload ile geliştirme sunucusu başlat (air gerekli)
	@which air > /dev/null || (echo "air kurulu değil. Kurmak için: go install github.com/air-verse/air@latest" && exit 1)
	air

build: ## Binary üret (bin/shade-backend)
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux go build \
		-ldflags="-s -w" \
		-o $(BUILD_DIR)/$(BINARY_NAME) \
		$(MAIN_PATH)
	@echo "✓ Build tamamlandı: $(BUILD_DIR)/$(BINARY_NAME)"

run: ## Sunucuyu doğrudan çalıştır (hot-reload yok)
	go run $(MAIN_PATH)

# ── Test & Kalite ─────────────────────────────────────────────────
test: ## Tüm testleri çalıştır (race detection dahil)
	go test ./... -v -race -count=1

test-short: ## Kısa testleri çalıştır (entegrasyon hariç)
	go test ./... -short -race

coverage: ## Test coverage raporu üret
	go test ./... -coverprofile=coverage.out -race
	go tool cover -html=coverage.out -o coverage.html
	@echo "✓ Rapor hazır: coverage.html"

lint: ## Lint kontrolü çalıştır
	@which golangci-lint > /dev/null || (echo "golangci-lint kurulu değil. Kurmak için: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run ./...

vet: ## Go vet çalıştır
	go vet ./...

security: ## Güvenlik taraması (govulncheck + gosec)
	@which govulncheck > /dev/null || go install golang.org/x/vuln/cmd/govulncheck@latest
	@which gosec > /dev/null || go install github.com/securego/gosec/v2/cmd/gosec@latest
	govulncheck ./...
	gosec -severity medium -exclude-dir=tools ./...

check: vet lint ## Vet + lint birlikte çalıştır
	@echo "✓ Tüm kontroller geçti"

# ── Docker ───────────────────────────────────────────────────────
docker-up: ## PostgreSQL + RabbitMQ başlat
	docker compose -f docker-compose.dev.yml up -d
	@echo "✓ Servisler başladı"
	@echo "  PostgreSQL : localhost:5432"
	@echo "  RabbitMQ  : localhost:5672 (yönetim: localhost:15672)"

docker-down: ## Tüm servisleri durdur
	docker compose -f docker-compose.dev.yml down

docker-logs: ## Servis loglarını takip et
	docker compose -f docker-compose.dev.yml logs -f

# ── Yardımcı ─────────────────────────────────────────────────────
clean: ## Build çıktılarını ve geçici dosyaları temizle
	rm -rf $(BUILD_DIR) coverage.out coverage.html
	@echo "✓ Temizlendi"

setup: ## Geliştirme araçlarını kur
	go install github.com/air-verse/air@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	@echo "✓ Tüm araçlar kuruldu"
	@echo ""
	@echo "Sonraki adım:"
	@echo "  1. cp .env.example .env"
	@echo "  2. .env dosyasını düzenle"
	@echo "  3. make docker-up"
	@echo "  4. make dev"
