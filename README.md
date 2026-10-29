# olist-marketplace

Dự án học microservices (Go) + BMAD, dùng dataset [Brazilian E-Commerce by Olist](https://www.kaggle.com/datasets/olistbr/brazilian-ecommerce).

## Cấu trúc

| Thư mục | Vai trò |
|---|---|
| `services/` | Mỗi service là một Go module riêng (`cmd/server`, `internal/`, `migrations/`, `Dockerfile`) |
| `proto/` | Hợp đồng gRPC dùng chung |
| `pkg/` | Thư viện dùng chung nhỏ: logger, tracing, eventbus |
| `deployments/` | `docker-compose.yml` chạy local, `k8s/` cho tuần 8 |
| `tools/olist-importer/` | Script nạp CSV Kaggle |
| `data/` | CSV Olist (không commit) |
| `docs/` | Tài liệu BMAD: brief, PRD, architecture, stories |

## Lệnh thường dùng

```bash
make build            # build mọi service vào ./bin
make run-catalog-service
make up               # docker compose
```
