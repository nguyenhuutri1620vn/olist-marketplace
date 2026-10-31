# olist-marketplace

Dự án học microservices (Go) + BMAD, dùng dataset [Brazilian E-Commerce by Olist](https://www.kaggle.com/datasets/olistbr/brazilian-ecommerce).

## Cấu trúc

| Thư mục | Vai trò |
|---|---|
| `services/` | 7 service, mỗi service là một Go module riêng: `api-gateway`, `identity-service`, `catalog-service`, `order-service`, `payment-service`, `shipping-simulator`, `analytics-service`. Service có dữ liệu dùng layout hexagonal (`cmd/server`, `internal/{domain,service,repository,transport}`, `migrations/`) |
| `proto/` | Hợp đồng gRPC `proto/<service>/v1`, kiểm tra bằng `buf`; code sinh ra nằm ở `proto/gen/go` |
| `contracts/events/v1/` | Danh mục event và command Kafka (nguồn duy nhất) |
| `pkg/` | Thư viện dùng chung: config, dbmigrate, bootstrap, logger, tracing, eventbus, outbox, inbox |
| `deployments/` | `docker-compose.yml` chạy local, init script Postgres (mỗi service một DB + user), `k8s/` cho tuần 8 |
| `tests/scaffold/` | Test kiểm tra cấu trúc repo khớp kiến trúc |
| `tools/olist-importer/` | Script nạp CSV Kaggle |
| `data/` | CSV Olist (không commit) |
| `docs/` | Tài liệu BMAD: brief, PRD, architecture, stories |

## Lệnh thường dùng

Cần Go 1.27 và Docker.

```bash
make build                # build mọi service vào ./bin
make test                 # test mọi service và module dùng chung
make proto                # buf generate + lint + breaking (so với develop)
make up                   # docker compose: 7 service + Postgres, Kafka, Jaeger, Prometheus, Grafana, kafka-ui
make import               # nạp Category, Seller, Product từ data/*.csv vào catalog (chạy lại không tạo trùng)
make check-db-isolation   # kiểm tra mỗi DB user chỉ vào được DB của mình
make down
```

Port mở ra host (chỉ trên `127.0.0.1`): api-gateway `8080`, Jaeger `16686`, Prometheus `9090`, Grafana `3000`, kafka-ui `8081`.

`make import` cần dataset Olist (Kaggle) trong `data/`. Importer chạy trong container trên mạng compose và chỉ gọi admin gRPC của catalog-service. Biến môi trường của importer: `DATA_DIR` (mặc định `/data`), `CATALOG_ADDR` (mặc định `catalog-service:9090`). Test `TestLoadRealDataset` chỉ chạy khi đặt `OLIST_DATA_DIR` trỏ tới thư mục CSV.

Init script Postgres chỉ chạy khi volume dữ liệu được tạo lần đầu. Đổi mật khẩu DB (`*_DB_PASSWORD`) hoặc thêm database thì phải xoá volume: `docker compose -f deployments/docker-compose.yml down -v`.
